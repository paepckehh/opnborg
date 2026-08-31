package opnborg

import (
	"html"
	"strings"
	"time"
)

// parseServerTag splits a "server[#asset-tag]" target entry into its host
// and optional asset tag. ok is false for malformed entries (empty host or
// more than one '#'), which the caller surfaces as a configuration error. A
// trailing "#" (empty tag) is tolerated and dropped. This helper is the
// single source of truth for the host/tag format; the status-tile init and
// the worker dispatch in srv() both parse through it so the two can never
// drift apart (see the AGENTS.md gotcha about the duplicated split).
func parseServerTag(server string) (host, tag string, ok bool) {
	s := strings.Split(server, "#")
	if len(s) == 2 && s[1] == "" {
		s = s[:1] // trailing "#": drop the empty tag
	}
	if len(s) > 2 || len(s[0]) == 0 {
		return "", "", false
	}
	if len(s) == 2 {
		return s[0], s[1], true
	}
	return s[0], "", true
}

// Start Server Application
func srv(config *OPNCall) error {
	// init
	var err error
	var servers []string

	// spin up Log/Display Engine
	display.Add(1)

	// spin up internal log / display engine
	go startLog(config)

	// startup app version & state, sleep panic gate
	suffix := "[CLI-ONE-TIME-PASS-MODE]"
	if config.Daemon {
		suffix = "[DAEMON-MODE][SLEEP:" + sleep + " SECONDS]"
	}
	displayChan <- []byte("[STARTING][" + _app + "][" + SemVer + "]" + suffix)

	// arm background timer
	go func() {
		// intit daily clock
		last, _, _ := time.Now().Clock()
		// loop forever
		for {
			time.Sleep(time.Duration(config.Sleep) * time.Second)
			updateOPN <- true
			now, _, _ := time.Now().Clock()
			// check for day rollover, perform unifi backup/export
			if now < last {
				if unifiBackupEnable.Load() {
					updateUnifiBackup <- true
				}
				if unifiExportEnable.Load() {
					updateUnifiExport <- true
				}
			}
			last, _, _ = time.Now().Clock()
		}
	}()

	// spin up internal webserver
	state := "[DISABLED]"
	if config.Httpd.Enable {
		go startWeb(config)
		state = "[ENABLED]"
	}
	displayChan <- []byte("[SERVICE][HTTPD]" + state + "[" + config.Httpd.Server + "]")

	// spin up internal rsyslog server
	state = "[DISABLED]"
	if config.RSysLog.Enable {
		go startRSysLog(config)
		state = "[ENABLED]"
	}
	displayChan <- []byte("[SERVICE][RSYSLOG]" + state)

	// spin up unifi backup server
	state = "[DISABLED]"
	if config.Unifi.Backup.Enable {
		state = "[ENABLED]"
		unifiStatus = "<div class=\"member-status\">" + _na + "</div><div class=\"member-main\"><span class=\"member-meta\">Member: " + config.Unifi.WebUI.String() + " Version: n/a Last Seen: n/a</span></div>"
		go srvUnifiBackup(config)
	}
	displayChan <- []byte("[SERVICE][UNIFI-BACKUP-AND-MONITORING]" + state)

	// spin up unifi asset export server
	state = "[DISABLED]"
	if config.Unifi.Export.Enable {
		state = "[ENABLED]"
		go srvUnifiExport(config)
	}
	displayChan <- []byte("[SERVICE][UNIFI-EXPORT-ASSET-INVENTORY]" + state)

	// spin up unifi autoBackup folder watch & sync server
	state = "[DISABLED]"
	if config.Unifi.Watch.Enable {
		state = "[ENABLED]"
		unifiWatchStatus = "<div class=\"member-status\">" + _na + "</div><div class=\"member-main\"><span class=\"member-meta\">Unifi autoBackup Watch: " + config.Unifi.Watch.Path + " Last Sync: n/a</span></div>"
		go srvUnifiWatch(config)
	}
	displayChan <- []byte("[SERVICE][UNIFI-WATCH-FOLDER-SYNC]" + state)

	// is opnsense hive is enabled?
	state = "[DISABLED]"
	if config.Enable {
		state = "[ENABLED]"
		// setup hive: build a single sanitized worker list first so the hive
		// status tiles and the worker pass indexes stay aligned. Invalid
		// entries (empty names, empty tags) are dropped here, not skipped
		// later; otherwise a targets string like "a,,b" would shift every
		// status tile and eventually panic on a hive index out of range.
		servers = nil
		for server := range strings.SplitSeq(config.Targets, ",") {
			host, tag, valid := parseServerTag(server)
			if !valid {
				displayChan <- []byte("[ERROR][CONFIGURATION] Line: " + server)
				continue
			}
			hive = append(hive, "<div class=\"member-status\">"+_na+"</div><div class=\"member-main\"><span class=\"member-meta\">Member: "+html.EscapeString(host)+" Version: n/a Last Seen: n/a</span></div>")
			if tag != "" {
				hive[len(hive)-1] += "<div class=\"meta-box meta-tag\"><span class=\"meta-label\">Tag</span><span class=\"meta-value\">" + html.EscapeString(tag) + "</span></div>"
			}
			servers = append(servers, server)
		}
	}
	displayChan <- []byte("[SERVICE][OPN-BACKUP-AND-MONITORING]" + state)

	// ensure the backup storage git repo is initialised before the first
	// worker pass runs, so the .git metadata and .gitignore exist up front.
	if config.Git.Enable {
		if err := gitInit(config); err != nil {
			displayChan <- []byte("[GIT][REPO][INIT][FAIL] " + err.Error())
			return err
		}
		displayChan <- []byte("[GIT][REPO][INIT][" + config.Path + "]")
	}

	// main loop
	for {
		// reset global (atomic) git worktree state tracker
		config.dirty.Store(false)

		// is opnsense hive is enabled
		if config.Enable {

			// fetch target configuration from master server
			if config.Sync.Enable {
				config.Sync.validConf = true
				config, err = readMasterConf(config)
				if err != nil {
					config.Sync.validConf = false
					displayChan <- []byte("[ERROR][UNABLE-TO-READ-MASTER-CONFIG]" + err.Error())
				}
			}

			// spinup individual worker for every server
			if config.Debug {
				displayChan <- []byte("[STARTING][BACKUP]")
			}
			// mark the backup pass as running so the forced-backup progress
			// dashboard (armed by /force via forceSeq) can stream live log
			// lines and detect completion.
			beginBackupPass()
			for id, server := range servers {
				host, tag, _ := parseServerTag(server) // validity was checked during hive init
				wg.Add(1)
				go actionOPN(host, tag, config, id, &wg)
			}

			// wait till all worker done
			wg.Wait()
			endBackupPass()
		}

		// check files into local git repo. gitCheckIn handles both the
		// dirty-worktree case (commit + push + gc) and the clean-worktree but
		// upstream-configured case (reconcile remote every pass so a previous
		// failed push or upstream drift is corrected), so a single call covers
		// both branches.
		if config.Git.Enable && (config.dirty.Load() || config.Git.Upstream != "") {
			if committed, err := gitCheckIn(config); err != nil {
				// a failed commit or push must never kill the daemon: the next
				// tick retries, and the failure is surfaced on the display engine
				// (and the WebUI dashboard git panel) for the operator.
				displayChan <- []byte("[GIT][REPO][CHECKIN][FAIL] " + err.Error())
			} else if committed {
				displayChan <- []byte("[CHANGES-DETECTED][GIT][REPO][CHECKIN][FINISH]")
			}
		}
		if config.dirty.Load() {
			displayChan <- []byte("[CHANGES-DETECTED][UPDATES-DONE][FINISH]")
		}

		// finish
		if config.Debug {
			displayChan <- []byte("[FINISH][BACKUP][ALL]")
		}

		// exit if not in daemon mode
		if !config.Daemon {
			// In one-shot mode the httpd, syslog, and unifi goroutines are
			// not armed, so no producer keeps sending to displayChan. The
			// channel is intentionally not closed (a close would race with
			// any lingering send from a unifi goroutine that was armed
			// before the one-shot gate); instead we return immediately and
			// the process exit terminates the startLog goroutine naturally.
			return nil
		}
		<-updateOPN
	}
}
