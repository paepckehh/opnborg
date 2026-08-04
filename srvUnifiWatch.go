package opnborg

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// global
const _uniWatch = "unifi-autobackup"

// srvUnifiWatch mirrors Unifi controller autoBackup files from a co-located
// source folder (OPN_UNIFI_WATCH_PATH, typically
// /var/lib/unifi/data/backup/autobackup) into the opnborg store. It is only
// armed at Setup() time when the folder exists and contains an
// autobackup_meta.json marker that parses as valid XML.
//
// On every watched change event (create/write/remove/rename) it copies the
// newest .unf file into the local backup store via checkIntoStore (which
// deduplicates by SHA-256 against the previous CONFIG-CURRENT), updates the
// watch status, and flags the git worktree dirty so the main loop commits.
func srvUnifiWatch(config *OPNCall) {

	// setup
	displayChan <- []byte("[UNIFI][WATCH][START][SOURCE] " + config.Unifi.Watch.Path)

	// initial sync so the store reflects the current source state immediately
	syncUnifiWatch(config, time.Now())

	// set status once from the initial pass
	setUnifiWatchStatus(config, true, true)

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
			ts := time.Now()
			timer = time.AfterFunc(debounce, func() {
				syncUnifiWatch(config, ts)
				setUnifiWatchStatus(config, true, true)
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
			ts := time.Now()
			syncUnifiWatch(config, ts)
			setUnifiWatchStatus(config, true, true)
		}
	}
}

// syncUnifiWatch copies the newest .unf file from the source autoBackup
// folder into the local store, deduplicated against the previous
// CONFIG-CURRENT by SHA-256 via checkIntoStore. The marker file mtime is
// refreshed into config.Unifi.Watch.LastTS so the WebUI can render the last
// sync date.
func syncUnifiWatch(config *OPNCall, ts time.Time) {

	// refresh marker mtime
	if fi, err := os.Stat(config.Unifi.Watch.Meta); err == nil {
		config.Unifi.Watch.LastTS = fi.ModTime()
	} else {
		// marker vanished: the controller may have rotated or removed it; keep
		// the last known timestamp and surface the condition in the log.
		displayChan <- []byte("[UNIFI][WATCH][WARN][META-FILE-MISSING] " + config.Unifi.Watch.Meta)
	}

	// find newest .unf file in the source folder
	entries, err := os.ReadDir(config.Unifi.Watch.Path)
	if err != nil {
		displayChan <- []byte("[UNIFI][WATCH][ERROR][READ-SOURCE-DIR] " + err.Error())
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
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mtime.After(files[j].mtime) })
	newest := files[0]

	// read newest backup file
	src := filepath.Join(config.Unifi.Watch.Path, newest.name)
	data, err := os.ReadFile(src)
	if err != nil {
		displayChan <- []byte("[UNIFI][WATCH][ERROR][READ-BACKUP-FILE] " + src + " " + err.Error())
		return
	}
	if len(data) < 1024 {
		displayChan <- []byte("[UNIFI][WATCH][ERROR][BACKUP-FILE-TOO-SMALL] " + src)
		return
	}

	// dedup against the previous current.unf via checkIntoStore (which writes
	// current.unf, the archive entry, the sha256.db line, and rotates the
	// CONFIG-CURRENT / CONFIG-LAST symlinks).
	sum := sha256.Sum256(data)
	if err := checkIntoStore(config, _uniWatch, "unf", data, ts, sum); err != nil {
		displayChan <- []byte("[UNIFI][WATCH][ERROR][STORE-CHECKIN] " + err.Error())
		return
	}

	// flag git worktree dirty so the main loop commits the mirrored backup
	config.dirty.Store(true)
	displayChan <- []byte("[UNIFI][WATCH][SYNC][SUCCESS] " + newest.name)
}
