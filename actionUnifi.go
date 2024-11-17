package opnborg

import (
	"sync"
	"time"
)

// actionUnifi, perform unifi backup
func actionUnifi(config *OPNCall, wg *sync.WaitGroup) {

	// setup
	defer wg.Done()
	// var err error
	if config.Debug {
		displayChan <- []byte("[UNIFI][BACKUP][START]" + config.Unifi.WebUI.Hostname())
	}

	// timestamp
	ts := time.Now()

	// get current opn config via xml
	fetchFail, degraded, notice := false, false, ""

	// fetch current unifi backup from server
	serverUnifi, err := fetchUnifi(config)
	if err != nil {
		displayChan <- []byte("[UNIFI][BACKUP][ERROR][FAIL:UNABLE-TO-FETCH-UNIFI] " + err.Error())
		// setOPNStatus(config, server, id, ts, notice, degraded, false)
		return
	}

	// check for changes
	// sum := sha256.Sum256(serverXML)
	// last := lastSum(config, server)
	// if sum == last {
	if config.Debug {
		displayChan <- []byte("[UNIFI][BACKUP][NO-CHANGE]")
	}
	//	setOPNStatus(config, server, id, ts, notice, degraded, true)
	//      return
	//}

	// set git global (atomic) worktree state tracker
	if config.Git {
		// config.dirty.Store(true)
	}

	// check xml file into storage
	// if err = checkIntoStore(config, server, serverXML, ts, sum); err != nil {
	//	displayChan <- []byte("[BACKUP][ERROR][FAIL:XML-STORE-CHECKIN] " + err.Error())
	//	setOPNStatus(config, server, id, ts, notice, degraded, false)
	//	return
	// }
	// displayChan <- []byte("[BACKUP][OK][SUCCESS:UNIFI-STORE-CHECKIN-OF-MODIFIED-XML]")
	// setOPNStatus(config, server, id, ts, notice, degraded, true)
	_ = serverUnifi
	_ = ts
	_ = degraded
	_ = notice
	_ = fetchFail
	return
}
