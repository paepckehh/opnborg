package opnborg

import (
	"crypto/sha256"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
)

// actionSrv, perform individual server backup
func actionSrv(server string, config *OPNCall, id int, wg *sync.WaitGroup) {

	// setup
	defer wg.Done()
	var err error
	if config.Debug {
		displayChan <- []byte("[BACKUP][START][SERVER] " + server)
	}

	// timestamp
	ts := time.Now()

	// skip the configuration & compliance section, till we have a valid master conf
	if config.Sync.validConf {

		// get current opn config via xml
		opn := new(Opnsense)
		if config.Sync.Enable || config.RSysLog.Enable {
			if opn, err = fetchOPN(server, config); err != nil {
				displayChan <- []byte("[XML][FAIL]" + err.Error())
			}
		}

		// check for pending BorgSYNC Orchestrator Tasks
		if server != config.Sync.Master {
			if err = checkInstallPKG(server, config, opn); err != nil {
				displayChan <- []byte("[SYNC][PKG][FAIL]" + err.Error())
			}
		}

		// check for pending BorgOPS Operations Tasks
		if config.RSysLog.Enable {
			if err = checkRSysLogConfig(server, config, opn); err != nil {
				displayChan <- []byte("[RSYSLOG][CLIENT-CONF][FAIL]" + err.Error())
			}
		}
	}

	// fetch current XML backup from server
	serverXML, err := fetchXML(server, config)
	if err != nil {
		displayChan <- []byte("[BACKUP][ERROR][FAIL:UNABLE-TO-FETCH] " + server)
		displayChan <- []byte("[BACKUP][ERROR][FAIL:UNABLE-TO-FETCH] " + err.Error())
		return
	}

	// backup was successful, update hive inventory
	seen := ts.Format(time.RFC3339) + " (" + humanize.Time(ts) + ")"
	version := getFirmwareVersion(config, server)
	status := "<b>Member: </b> " + server + " <b>Version: </b>" + version + " <b>Last Seen: </b>" + seen + "<br>"
	hiveMutex.Lock()
	hive[id] = status
	hiveMutex.Unlock()

	// check for changes
	sum := sha256.Sum256(serverXML)
	last := lastSum(config, server)
	if sum == last {
		if config.Debug {
			displayChan <- []byte("[BACKUP][SERVER][NO-CHANGE] " + server)
		}
		return
	}

	// set git global (atomic) worktree state tracker
	if config.Git {
		config.dirty.Store(true)
	}

	// check xml file into storage
	if err = checkIntoStore(config, server, serverXML, ts, sum); err != nil {
		displayChan <- []byte("[BACKUP][ERROR][FAIL:XML-STORE-CHECKIN] " + err.Error())
		return
	}
	displayChan <- []byte("[BACKUP][OK][SUCCESS:XML-STORE-CHECKIN-OF-MODIFIED-XML]")
}
