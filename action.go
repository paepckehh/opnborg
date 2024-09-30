package opnborg

import (
	"crypto/sha256"
	"path/filepath"
	"strconv"
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
	opn := new(Opnsense)
	if config.Sync.Enable || config.RSysLog.Enable {
		if opn, err = fetchOPN(server, config); err != nil {
			displayChan <- []byte("[XML][FAIL]" + err.Error())
		}
	} else {

		// check for pending BorgSYNC Orchestrator Tasks
		if config.Sync.validConf && server != config.Sync.Master {
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
	year, month, _ := ts.Date()
	archive := filepath.Join(_archive, strconv.Itoa(year), padMonth(strconv.Itoa(int(month))))
	seen := ts.Format(time.RFC3339)
	version := getFirmwareVersion(config, server)
	linkCurrent := "<a href=\"./files/" + server + "/current.xml\"><button type=\"button\"><b>[current.xml]</b></button></a>"
	linkArchive := "<a href=\"./files/" + server + "/" + archive + "\"><button type=\"button\"><b>[archive]</b></button></a>"
	links := linkCurrent + " " + linkArchive
	status := _ok + " <b>Member: </b> " + server + " <b>Version: </b>" + version + " <b>Last Seen: </b>" + seen + " <b>Files: </b>" + links + "<br>"
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
