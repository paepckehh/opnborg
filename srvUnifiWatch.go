package opnborg

import (
	"crypto/sha256"
	"encoding/base64"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// global
const _uniWatch = "unifi-autobackup"

// srvUnifiWatch mirrors Unifi controller autoBackup files from a co-located
// source folder (OPN_UNIFI_WATCH_PATH, typically
// /var/lib/unifi/data/backup/autobackup) into the opnborg store. It is only
// armed at Setup() time when the folder exists and contains a readable
// autobackup_meta.json marker file (existence + read access only; the marker
// contents are not parsed or validated).
//
// On every watched change event (create/write/remove/rename) it copies the
// newest .unf file into the local backup store via checkIntoStore (which
// deduplicates by SHA-256 against the previous CONFIG-CURRENT), updates the
// watch status, and flags the git worktree dirty so the main loop commits.
func srvUnifiWatch(config *OPNCall) {

	// setup
	displayChan <- []byte("[UNIFI][WATCH][START][SOURCE] " + config.Unifi.Watch.Path)

	// initial sync so the store reflects the current source state immediately
	syncUnifiWatch(config)

	// set status once from the initial pass (degraded if the sync failed)
	setUnifiWatchStatus(config, true, lastUnifiWatchSyncOK(config))

	// setup fsnotify watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		displayChan <- []byte("[UNIFI][WATCH][ERROR][WATCHER-SETUP-FAIL] " + err.Error())
		setUnifiWatchStatus(config, true, false)
		return
	}
	defer watcher.Close()
	if err := watcher.Add(config.Unifi.Watch.Path); err != nil {
		displayChan <- []byte("[UNIFI][WATCH][ERROR][WATCHER-ADD-FAIL] " + err.Error())
		setUnifiWatchStatus(config, true, false)
		return
	}

	// debounce rapid event bursts (a controller backup run emits several
	// create/write events in quick succession) into a single sync pass.
	const debounce = 2 * time.Second
	var timer *time.Timer

	// loop forever
	for {
		select {
		case ev, ok := <-watcher.Events:
			if !ok {
				return
			}
			// only react to .unf backup files or the marker file
			if !strings.HasSuffix(ev.Name, ".unf") &&
				!strings.HasSuffix(filepath.Base(ev.Name), "autobackup_meta.json") {
				continue
			}
			// (re)arm the debounce timer
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(debounce, func() {
				syncUnifiWatch(config)
				setUnifiWatchStatus(config, true, lastUnifiWatchSyncOK(config))
				// allow the main loop to poke the next sync cycle on /force
				select {
				case updateUnifiWatch <- true:
				default:
				}
			})
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			displayChan <- []byte("[UNIFI][WATCH][ERROR][WATCHER-EVENT] " + err.Error())
			setUnifiWatchStatus(config, true, false)
		case <-updateUnifiWatch:
			// manual trigger (/force) or daily rollover
			syncUnifiWatch(config)
			setUnifiWatchStatus(config, true, lastUnifiWatchSyncOK(config))
		}
	}
}

// syncUnifiWatch copies every .unf file from the source autoBackup folder
// into the local store, deduplicated against the previous current.unf by
// SHA-256 via checkIntoStore (each new file gets its own archive entry; the
// newest file wins the current.unf pointer). It records per-pass stats
// (files seen / synced / skipped, last synced file name, last sync timestamp
// and any error reason) on config.Unifi.Watch under unifiWatchMutex so the
// main WebUI tile and the config-dashboard Unifi panel can surface them.
func syncUnifiWatch(config *OPNCall) {

	// refresh marker mtime
	if fi, err := os.Stat(config.Unifi.Watch.Meta); err == nil {
		config.Unifi.Watch.LastTS = fi.ModTime()
	} else {
		// marker vanished: the controller may have rotated or removed it; keep
		// the last known timestamp and surface the condition in the log.
		displayChan <- []byte("[UNIFI][WATCH][WARN][META-FILE-MISSING] " + config.Unifi.Watch.Meta)
	}

	// find all .unf files in the source folder
	entries, err := os.ReadDir(config.Unifi.Watch.Path)
	if err != nil {
		reason := "READ-SOURCE-DIR: " + err.Error()
		setUnifiWatchSyncResult(config, reason, 0, 0, 0, "")
		displayChan <- []byte("[UNIFI][WATCH][ERROR][" + reason + "]")
		return
	}
	type candidate struct {
		name  string
		mtime time.Time
	}
	var files []candidate
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".unf") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, candidate{name: e.Name(), mtime: fi.ModTime()})
	}
	if len(files) == 0 {
		displayChan <- []byte("[UNIFI][WATCH][INFO][NO-BACKUP-FILES] " + config.Unifi.Watch.Path)
		setUnifiWatchSyncResult(config, "", 0, 0, 0, "")
		return
	}
	// oldest -> newest so the newest file wins the current.unf pointer
	sort.Slice(files, func(i, j int) bool { return files[i].mtime.Before(files[j].mtime) })

	// baseline: the sha256 of the existing current.unf (if any) so we only
	// checkin files that actually differ from what is already stored.
	currentSum := lastSum(config, _uniWatch)
	// build a dedup set of every checksum already recorded in the per-server
	// sha256.db (plus current.unf) so re-sync passes skip files already
	// archived rather than creating duplicate archive entries.
	archived := archivedSums(config, _uniWatch)
	archived[currentSum] = true

	synced, skipped := 0, 0
	lastFile := ""
	var firstErr string
	for _, f := range files {
		src := filepath.Join(config.Unifi.Watch.Path, f.name)
		data, err := os.ReadFile(src)
		if err != nil {
			reason := "READ-BACKUP-FILE: " + src + " " + err.Error()
			if firstErr == "" {
				firstErr = reason
			}
			displayChan <- []byte("[UNIFI][WATCH][ERROR][" + reason + "]")
			continue
		}
		if len(data) < 1024 {
			reason := "BACKUP-FILE-TOO-SMALL: " + src + " (" + strconv.Itoa(len(data)) + " bytes)"
			if firstErr == "" {
				firstErr = reason
			}
			displayChan <- []byte("[UNIFI][WATCH][ERROR][" + reason + "]")
			continue
		}
		sum := sha256.Sum256(data)
		if archived[sum] {
			skipped++
			lastFile = f.name
			continue
		}
		// use the file's own mtime for the archive entry so multiple files
		// checked in during the same pass get distinct archive names.
		if err := checkIntoStore(config, _uniWatch, "unf", data, f.mtime, sum); err != nil {
			reason := "STORE-CHECKIN: " + f.name + " " + err.Error()
			if firstErr == "" {
				firstErr = reason
			}
			displayChan <- []byte("[UNIFI][WATCH][ERROR][" + reason + "]")
			continue
		}
		archived[sum] = true
		currentSum = sum
		synced++
		lastFile = f.name
	}

	// flag git worktree dirty so the main loop commits the mirrored backup(s)
	if synced > 0 {
		config.dirty.Store(true)
	}
	setUnifiWatchSyncResult(config, firstErr, len(files), synced, skipped, lastFile)
	if firstErr != "" {
		displayChan <- []byte("[UNIFI][WATCH][SYNC][PARTIAL] synced=" + strconv.Itoa(synced) + " skipped=" + strconv.Itoa(skipped) + " err=" + firstErr)
	} else if synced > 0 {
		displayChan <- []byte("[UNIFI][WATCH][SYNC][SUCCESS] synced=" + strconv.Itoa(synced) + " skipped=" + strconv.Itoa(skipped) + " last=" + lastFile)
	}
}

// setUnifiWatchSyncResult records the outcome of a sync pass on the config
// struct under unifiWatchMutex so the WebUI and config-dashboard render paths
// see a consistent snapshot.
func setUnifiWatchSyncResult(config *OPNCall, errReason string, source, synced, skipped int, lastFile string) {
	unifiWatchMutex.Lock()
	defer unifiWatchMutex.Unlock()
	config.Unifi.Watch.LastSyncTS = time.Now()
	config.Unifi.Watch.LastSyncErr = errReason
	config.Unifi.Watch.SourceFiles = source
	config.Unifi.Watch.SyncedFiles = synced
	config.Unifi.Watch.SkippedFiles = skipped
	config.Unifi.Watch.LastFile = lastFile
}

// lastUnifiWatchSyncOK reports whether the most recent sync pass recorded no
// error. It reads the shared field under unifiWatchMutex so it is safe to call
// from the watcher goroutine right after a sync pass.
func lastUnifiWatchSyncOK(config *OPNCall) bool {
	unifiWatchMutex.Lock()
	defer unifiWatchMutex.Unlock()
	return config.Unifi.Watch.LastSyncErr == ""
}

// archivedCount returns the number of backup files currently held in the
// per-server store, derived from the append-only sha256.db log. It is used by
// the WebUI unifi watch tile to surface a live total of stored backups.
func archivedCount(config *OPNCall, server string) int {
	data, err := os.ReadFile(filepath.Join(config.Path, server, _hashFile))
	if err != nil {
		return 0
	}
	count := 0
	for line := range strings.SplitSeq(string(data), _linefeed) {
		if _, digest, found := strings.Cut(line, _tab); found && digest != "" {
			count++
		}
	}
	return count
}

// archivedSums returns the set of base64 SHA-256 digests recorded in the
// per-server sha256.db log. It lets syncUnifiWatch dedup every source file
// against the full archive history (not only current.<ext>) so re-sync passes
// skip files that are already stored instead of creating duplicate archive
// entries. The map is empty when no archive exists yet.
func archivedSums(config *OPNCall, server string) map[[32]byte]bool {
	out := make(map[[32]byte]bool)
	data, err := os.ReadFile(filepath.Join(config.Path, server, _hashFile))
	if err != nil {
		return out
	}
	for line := range strings.SplitSeq(string(data), _linefeed) {
		_, digest, found := strings.Cut(line, _tab)
		if !found || digest == "" {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(digest)
		if err != nil || len(raw) != 32 {
			continue
		}
		var sum [32]byte
		copy(sum[:], raw)
		out[sum] = true
	}
	return out
}
