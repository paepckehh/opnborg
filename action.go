package opnborg

import (
	"crypto/sha256"
	"sync"
	"time"
)

// actionSrv, perform individual server backup
func actionSrv(server string, config *OPNCall, wg *sync.WaitGroup) {

	// setup
	defer wg.Done()
	var err error
	if config.Debug {
		displayChan <- []byte("[BACKUP][START][SERVER] " + server)
	}

	// get current opn config via xml
	opn := new(Opnsense)
	if config.Sync.Enable || config.RSysLog.Enable {
		if opn, err = fetchOPN(server, config); err != nil {
			displayChan <- []byte("[XML][FAIL] " + server)
			displayChan <- []byte("[XML][FAIL] " + err.Error())
		}
	}

	// check for pending BorgSYNC Orchestrator Tasks
	if config.Sync.Enable {
		if server != config.Sync.Master {
			if err = checkInstallPKG(server, config, opn); err != nil {
				displayChan <- []byte("[SYNC][PKG][FAIL] " + server)
				displayChan <- []byte("[SYNC][PKG][FAIL] " + err.Error())
			}
		}
	}

	// check for pending BorgOPS Operations Tasks
	if config.RSysLog.Enable {
		if err = checkRSysLogConfig(server, config, opn); err != nil {
			displayChan <- []byte("[RSYSLOG][CLIENT-CONF][FAIL] " + server)
			displayChan <- []byte("[RSYSLOG][CLIENT-CONF][FAIL] " + err.Error())
		}
	}

	// timestamp
	ts := time.Now()

	// fetch current XML backup from server
	serverXML, err := fetchXML(server, config)
	if err != nil {
		displayChan <- []byte("[BACKUP][ERROR][FAIL:UNABLE-TO-FETCH] " + server)
		displayChan <- []byte("[BACKUP][ERROR][FAIL:UNABLE-TO-FETCH] " + err.Error())
		return
	}

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
