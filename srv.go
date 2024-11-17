package opnborg

import (
	"strings"
	"time"
)

// Start Server Application
func srv(config *OPNCall) error {
	// init
	var err error

	// spin up Log/Display Engine
	display.Add(1)

	// spin up internal log / display engine
	go startLog(config)

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

	// setup hive
	servers := strings.Split(config.Targets, ",")
	for _, server := range servers {
		status := _na + " <b>Member: </b> " + server + " <b>Version: </b>n/a <b>Last Seen: </b>n/a<br>"
		hive = append(hive, status)
	}

	// startup app version & state, sleep panic gate
	suffix := "[CLI-ONE-TIME-PASS-MODE]"
	if config.Daemon {
		suffix = "[DAEMON-MODE][SLEEP:" + sleep + " SECONDS]"
	}
	displayChan <- []byte("[STARTING][" + _app + "][" + SemVer + "]" + suffix)

	// spin up timer
	go func() {
		time.Sleep(time.Duration(config.Sleep) * time.Second)
		update <- true
	}()

	// main loop
	for {

		// fetch target configuration from master server
		if config.Sync.Enable {
			config.Sync.validConf = true
			config, err = readMasterConf(config)
			if err != nil {
				config.Sync.validConf = false
				displayChan <- []byte("[ERROR][UNABLE-TO-READ-MASTER-CONFIG]" + err.Error())
			}
		}

		// reset global (atomic) git worktree state tracker
		if config.Git {
			config.dirty.Store(false)
		}

		// spinup individual worker for every server
		if config.Debug {
			displayChan <- []byte("[STARTING][BACKUP]")
		}
		for id, server := range servers {
			wg.Add(1)
			go actionOPN(server, config, id, &wg)
		}

		// spinup unifi backup engine
		if config.Unifi.Backup.Enable {
			wg.Add(1)
			go actionUnifi(config, &wg)
		}

		// wait till all worker done
		wg.Wait()

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

		// set loop wait
		// select {
		// case <-update:
		//	break
		// }
		<-update
	}
}
