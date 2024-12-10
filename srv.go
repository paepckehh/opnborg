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
		time.Sleep(time.Duration(config.Sleep) * time.Second)
		updateOPN <- true
		if unifiEnable.Load() {
			updateUnifi <- true
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
		unifiStatus = _na + " <b>Member: </b> " + config.Unifi.WebUI.String() + " <b>Version: </b>n/a <b>Last Seen: </b>n/a<br>"
		go unifiBackupServer(config)
	}
	displayChan <- []byte("[SERVICE][UNIFI-BACKUP-AND-MONITORING]" + state)

	// is opnsense hive is enabled?
	state = "[DISABLED]"
	if config.Enable {
		state = "[ENABLED]"
		// setup hive
		servers = strings.Split(config.Targets, ",")
		for _, server := range servers {
			status := _na + " <b>Member: </b> " + server + " <b>Version: </b>n/a <b>Last Seen: </b>n/a<br>"
			hive = append(hive, status)
		}
	}
	displayChan <- []byte("[SERVICE][OPN-BACKUP-AND-MONITORING]" + state)

	// main loop
	for {
		// reset global (atomic) git worktree state tracker
		if config.Git {
			config.dirty.Store(false)
		}

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
			for id, server := range servers {
				wg.Add(1)
				go actionOPN(server, config, id, &wg)
			}

			// wait till all worker done
			wg.Wait()
		}

		// check files into local git repo
		if config.dirty.Load() {
			if config.Git {
				if err := gitCheckIn(config); err != nil {
					displayChan <- []byte("[GIT][REPO][CHECKIN][FAIL]")
					return err
				}
				displayChan <- []byte("[CHANGES-DETECTED][GIT][REPO][CHECKIN][FINISH]")
			}
			displayChan <- []byte("[CHANGES-DETECTED][UPDATES-DONE][FINISH]")
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
