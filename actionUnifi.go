package opnborg

import (
	"sync"
)

// actionUnifi, perform unifi backup
func actionUnifi(config *OPNCall, wg *sync.WaitGroup) {

	// setup
	defer wg.Done()
	// var err error
	if config.Debug {
		displayChan <- []byte("[BACKUP][START][UNIFI]")
	}

	// timestamp
	// ts := time.Now()

	// get current opn config via xml
	// fetchFail, degraded, notice := false, false, ""

	// fetch current XML backup from server
	// serverUnifi, err := fetchUnifi(config)
	// if err != nil {
	//	displayChan <- []byte("[BACKUP][ERROR][FAIL:UNABLE-TO-FETCH-UNIFI] "+ err.Error())
	//	setOPNStatus(config, server, id, ts, notice, degraded, false)
	//	return
	// }

	// check for changes
	// sum := sha256.Sum256(serverXML)
	// last := lastSum(config, server)
	// if sum == last {
	if config.Debug {
		displayChan <- []byte("[BACKUP][UNIFI][NO-CHANGE]")
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
	return
}
