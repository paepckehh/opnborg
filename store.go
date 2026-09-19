package opnborg

import (
	"crypto/sha256"
	"encoding/base64"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// const
const (
	_archive  = ".archive"
	_tab      = "	"
	_linefeed = "\n"
	_current  = "CONFIG-CURRENT"
	_last     = "CONFIG-LAST"
	_hashFile = "sha256.db"

	// _minBackupBytes is the lower bound a downloaded Unifi .unf backup must
	// reach before it is accepted into the store; anything smaller is an
	// error page or an empty placeholder, never a valid backup.
	_minBackupBytes = 1024
)

// logBackupErr reports a backup-store failure to the display engine. It
// centralises the repeated `displayChan <- []byte("[BACKUP][ERROR]... ")`
// pattern used across checkIntoStore and lastSum without altering the error
// value each caller returns.
func logBackupErr(msg, ctx string) {
	displayChan <- []byte("[BACKUP][ERROR][" + msg + "] " + ctx)
}

// lastSum check last XML file sha256 checksum
func lastSum(config *OPNCall, server string) [32]byte {
	data, err := os.ReadFile(filepath.Join(config.Path, server, _current))
	if err != nil {
		if !os.IsNotExist(err) {
			logBackupErr("FAIL:UNABLE-TO-READ-HASHSHUM-FILE", server)
		}
		return [32]byte{}
	}
	return sha256.Sum256(data)
}

// writeAtomic writes data to path through a temporary file created in tmpDir
// and a final rename (which may cross directories but must stay on the same
// filesystem, so tmpDir is always a subdir of the store tree). The storage
// tree is read concurrently by the httpd file server (current.xml/current.unf
// downloads) and by go-git staging during gitCheckIn, so a truncate-in-place
// os.WriteFile would let a reader observe a partially written (or empty)
// backup; the rename makes the new content appear atomically. tmpDir is the
// gitignored .archive subtree so a temp file orphaned by a crash can never
// surface in the git worktree status and enter a commit.
func writeAtomic(tmpDir, path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(tmpDir, ".store-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		// empty tmpName marks a successful rename: nothing left to clean up.
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	tmpName = ""
	return nil
}

// checkIntoStore writes a new backup payload into the per-server archive tree,
// rotates the current.xml/CONFIG-CURRENT/CONFIG-LAST pointers, and appends the
// SHA-256 entry to sha256.db. All paths are resolved against config.Path so
// the function does NOT call os.Chdir — the process working directory is a
// shared resource and the OPN backup workers call this concurrently, so Chdir
// here would race with the git and httpd goroutines. The CONFIG-CURRENT
// symlink target stays relative (archiveFile is already relative to the
// server dir) so the store remains portable when copied/moved.
func checkIntoStore(config *OPNCall, server, ext string, serverXML []byte, ts time.Time, sum [32]byte) error {
	ext = "." + ext
	year, month, _ := ts.Date()

	// per-server archive subtree: <OPN_PATH>/<server>/.archive/<YYYY>/<MM>/
	store := filepath.Join(_archive, strconv.Itoa(year), padMonth(strconv.Itoa(int(month))))
	serverRoot := filepath.Join(config.Path, server)
	fullPath := filepath.Join(serverRoot, store)
	if err := os.MkdirAll(fullPath, 0770); err != nil {
		logBackupErr("FAIL:UNABLE-TO-CREATE-FILE-STORAGE", fullPath)
		return err
	}

	// write the timestamped archive entry. Millisecond precision avoids
	// archive-name collisions when several distinct payloads are checked in
	// during the same second (e.g. the initial Unifi autoBackup sync pass
	// mirroring multiple pre-existing .unf files at once).
	name := ts.UTC().Format("20060102T150405.000Z") + "-" + server + ext
	archiveRel := filepath.Join(store, name) // relative to serverRoot
	archiveAbs := filepath.Join(serverRoot, archiveRel)
	if err := writeAtomic(fullPath, archiveAbs, serverXML, 0660); err != nil {
		logBackupErr("FAIL:UNABLE-TO-CREATE-ARCHIVE-FILE", server)
		return err
	}

	// refresh the regular current.<ext> file (served by the WebUI /files/
	// handler and hashed by lastSum). Written atomically: the rename replaces
	// any previous file in one step, so readers never see a partial write or
	// a missing-file window.
	currentFile := filepath.Join(serverRoot, "current"+ext)
	if err := writeAtomic(fullPath, currentFile, serverXML, 0660); err != nil {
		logBackupErr("FAIL:UNABLE-TO-CREATE-CURRENT-FILE", archiveRel)
		return err
	}

	// rotate the CONFIG-LAST / CONFIG-CURRENT symlinks. The symlink target
	// stays relative (archiveRel) so the store tree is portable. Remove
	// pre-existing links before rename/create to avoid EEXIST on filesystems
	// that do not overwrite symlinks in place.
	currentLink := filepath.Join(serverRoot, _current)
	lastLink := filepath.Join(serverRoot, _last)
	_ = os.Remove(lastLink)
	_ = os.Rename(currentLink, lastLink)
	if err := os.Symlink(archiveRel, currentLink); err != nil {
		logBackupErr("FAIL:UNABLE-TO-CREATE-ARCHIVE-SYMLINK", server)
		return err
	}

	// append the sha256.db entry. This runs last, after the archive file,
	// the current.<ext> file, and the symlink rotation have all succeeded:
	// the hash log doubles as the "already archived" dedup set for the
	// unifi autoBackup watch sync, so recording a payload whose rotation
	// failed would make every later pass skip re-checking it in while
	// CONFIG-CURRENT keeps pointing at the older payload.
	logEntry := name + _tab + base64.StdEncoding.EncodeToString(sum[:]) + _linefeed
	hashPath := filepath.Join(serverRoot, _hashFile)
	hashFile, err := os.OpenFile(hashPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0660)
	if err != nil {
		logBackupErr("FAIL:UNABLE-TO-OPEN-OR-CREATE-HASHSHUM-FILE", server)
		return err
	}
	if _, err := hashFile.Write([]byte(logEntry)); err != nil {
		logBackupErr("FAIL:UNABLE-TO-WRITE-TO-HASHSHUM-FILE", server)
		_ = hashFile.Close()
		return err
	}
	if err := hashFile.Close(); err != nil {
		logBackupErr("FAIL:UNABLE-TO-SAVE-HASHSHUM-FILE", server)
		return err
	}
	return nil
}
