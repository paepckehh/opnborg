package opnborg

import (
	"strings"
	"time"
)

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
		// setup hive
		servers = strings.Split(config.Targets, ",")
		for _, server := range servers {
			s := strings.Split(server, "#")
			switch len(s) {
			case 1:
				if len(s[0]) > 0 {
					status := "<div class=\"member-status\">" + _na + "</div><div class=\"member-main\"><span class=\"member-meta\">Member: " + s[0] + " Version: n/a Last Seen: n/a</span></div>"
					hive = append(hive, status)
				}
			case 2:
				if len(s[0]) > 0 && len(s[1]) > 0 {
					status := "<div class=\"member-status\">" + _na + "</div><div class=\"member-main\"><span class=\"member-meta\">Member: " + s[0] + " Version: n/a Last Seen: n/a</span></div><div class=\"meta-box meta-tag\"><span class=\"meta-label\">Tag</span><span class=\"meta-value\">" + s[1] + "</span></div>"
					hive = append(hive, status)
				}
			default:
				status := "<div class=\"member-status\">" + _na + "</div><div class=\"member-main\"><span class=\"member-meta\">configuration error, please fix configuration line for server: " + server + "</span></div>"
				hive = append(hive, status)
			}
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
				s := strings.Split(server, "#")
				switch len(s) {
				case 1:
					wg.Add(1)
					go actionOPN(s[0], "", config, id, &wg)
				case 2:
					wg.Add(1)
					go actionOPN(s[0], s[1], config, id, &wg)
				default:
					displayChan <- []byte("[ERROR][CONFIGURATION] Line: " + server)
				}

			}

			// wait till all worker done
			wg.Wait()
			endBackupPass()
		}

		// check files into local git repo
		if config.dirty.Load() {
			if config.Git.Enable {
				if committed, err := gitCheckIn(config); err != nil {
					displayChan <- []byte("[GIT][REPO][CHECKIN][FAIL] " + err.Error())
					return err
				} else if committed {
					displayChan <- []byte("[CHANGES-DETECTED][GIT][REPO][CHECKIN][FINISH]")
				}
			}
			displayChan <- []byte("[CHANGES-DETECTED][UPDATES-DONE][FINISH]")
		} else if config.Git.Enable && config.Git.Upstream != "" {
			// No local changes this tick, but an upstream is configured: still
			// reconcile the remote so a previous failed push or an upstream drift
			// is corrected every pass (force-push via the configured refspec).
			if committed, err := gitCheckIn(config); err != nil {
				displayChan <- []byte("[GIT][REPO][CHECKIN][FAIL] " + err.Error())
				return err
			} else if committed {
				displayChan <- []byte("[CHANGES-DETECTED][GIT][REPO][CHECKIN][FINISH]")
			}
		}

		// finish
		if config.Debug {
			displayChan <- []byte("[FINISH][BACKUP][ALL]")
		}

		// exit if not in daemon mode
		if !config.Daemon {
			close(displayChan)
			display.Wait()
			return nil
		}
		<-updateOPN
	}
}
