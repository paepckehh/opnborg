package opnborg

import (
	"crypto/sha256"
	"path/filepath"
	"strconv"
	"strings"
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
	fetchFail, degraded := false, false
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
			displayChan <- []byte("[RSYSLOG][CLIENT-CONF][FAIL]" + err.Error())
			degraded = true
		}
	}

	// fetch current XML backup from server
	serverXML, err := fetchXML(server, config)
	if err != nil {
		displayChan <- []byte("[BACKUP][ERROR][FAIL:UNABLE-TO-FETCH-XML] " + server + err.Error())
		setOPNStatus(config, server, id, ts, degraded, false)
		return
	}

	// check for changes
	sum := sha256.Sum256(serverXML)
	last := lastSum(config, server)
	if sum == last {
		if config.Debug {
			displayChan <- []byte("[BACKUP][SERVER][NO-CHANGE] " + server)
		}
		setOPNStatus(config, server, id, ts, degraded, true)
		return
	}

	// set git global (atomic) worktree state tracker
	if config.Git {
		config.dirty.Store(true)
	}

	// check xml file into storage
	if err = checkIntoStore(config, server, serverXML, ts, sum); err != nil {
		displayChan <- []byte("[BACKUP][ERROR][FAIL:XML-STORE-CHECKIN] " + err.Error())
		setOPNStatus(config, server, id, ts, degraded, false)
		return
	}
	displayChan <- []byte("[BACKUP][OK][SUCCESS:XML-STORE-CHECKIN-OF-MODIFIED-XML]")
	setOPNStatus(config, server, id, ts, degraded, true)
}

// setOPNStatus
func setOPNStatus(config *OPNCall, server string, id int, ts time.Time, degraded, ok bool) {
	year, month, _ := ts.Date()
	archive := filepath.Join(_archive, strconv.Itoa(year), padMonth(strconv.Itoa(int(month))))
	if ok {
		state := _ok
		if degraded {
			state = _degraded
		}
		seen := ts.Format(time.RFC3339)
		ver := getFirmwareVersion(config, server)
		linkCurrent := "<a href=\"./files/" + server + "/current.xml\"><button type=\"button\"><b>[current.xml]</b></button></a>"
		linkArchive := "<a href=\"./files/" + server + "/" + archive + "\"><button type=\"button\"><b>[archive]</b></button></a>"
		links := linkCurrent + " " + linkArchive
		status := state + " <b>Member: </b> " + server + " <b>Version: </b>" + ver + " <b>Last Seen: </b>" + seen + " <b>Files: </b>" + links + "<br>"
		hiveMutex.Lock()
		hive[id] = status
		hiveMutex.Unlock()
		return
	}
	hiveMutex.Lock()
	status := hive[id]
	status = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(status, _ok, ""), _na, ""), _fail, ""), _degraded, "")
	status = _fail + status
	hive[id] = status
	hiveMutex.Unlock()
	return
}
