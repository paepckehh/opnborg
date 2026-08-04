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
	if err := os.WriteFile(archiveAbs, serverXML, 0660); err != nil {
		logBackupErr("FAIL:UNABLE-TO-CREATE-ARCHIVE-FILE", server)
		return err
	}

	// refresh the regular current.<ext> file (served by the WebUI /files/ handler)
	currentFile := filepath.Join(serverRoot, "current"+ext)
	_ = os.Remove(currentFile)
	if err := os.WriteFile(currentFile, serverXML, 0660); err != nil {
		logBackupErr("FAIL:UNABLE-TO-CREATE-CURRENT-FILE", archiveRel)
		return err
	}

	// append the sha256.db entry
	logEntry := name + _tab + base64.StdEncoding.EncodeToString(sum[:]) + _linefeed
	hashPath := filepath.Join(serverRoot, _hashFile)
	hashFile, err := os.OpenFile(hashPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
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
	return nil
}
