package opnborg

import (
	"crypto/sha256"
	"sync"
	"time"
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

	// get current opn config via xml
	fetchFail, degraded, notice := false, false, ""
	opn := new(Opnsense)
	if config.Sync.Enable || config.RSysLog.Enable {
		if opn, err = fetchOPN(server, config); err != nil {
			displayChan <- []byte("[XML][FAIL]" + err.Error())
			degraded = true
			fetchFail = true
		}
	}

	// check for pending BorgSYNC Orchestrator Tasks
	if config.Sync.validConf && server != config.Sync.Master && !fetchFail {
		if err = checkInstallPKG(server, config, opn); err != nil {
			displayChan <- []byte("[SYNC][PKG][FAIL]" + err.Error())
			degraded = true
		}
	}

	// check for pending BorgOPS Operations Tasks
	if config.RSysLog.Enable && !fetchFail {
		if err = checkRSysLogConfig(server, config, opn); err != nil {
			notice = "[RSYSLOG][CLIENT-CONF][FAIL]" + err.Error()
			displayChan <- []byte(notice)
			degraded = true
		}
	}

	// fetch current XML backup from server
	serverXML, err := fetchXML(server, config)
	if err != nil {
		displayChan <- []byte("[BACKUP][ERROR][FAIL:UNABLE-TO-FETCH-XML] " + server + err.Error())
		setOPNStatus(config, server, id, ts, notice, degraded, false)
		return
	}

	// check for changes
	sum := sha256.Sum256(serverXML)
	last := lastSum(config, server)
	if sum == last {
		if config.Debug {
			displayChan <- []byte("[BACKUP][SERVER][NO-CHANGE] " + server)
		}
		setOPNStatus(config, server, id, ts, notice, degraded, true)
		return
	}

	// set git global (atomic) worktree state tracker
	if config.Git {
		config.dirty.Store(true)
	}

	// check xml file into storage
	if err = checkIntoStore(config, server, serverXML, ts, sum); err != nil {
		displayChan <- []byte("[BACKUP][ERROR][FAIL:XML-STORE-CHECKIN] " + err.Error())
		setOPNStatus(config, server, id, ts, notice, degraded, false)
		return
	}
	displayChan <- []byte("[BACKUP][OK][SUCCESS:XML-STORE-CHECKIN-OF-MODIFIED-XML]")
	setOPNStatus(config, server, id, ts, notice, degraded, true)
}
