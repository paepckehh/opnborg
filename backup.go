package opnborg

import (
	"crypto/sha256"
	"sync"
	"time"
)

// backupSrv, perform individual server backup
func backupSrv(server string, config *OPNCall, wg *sync.WaitGroup) {

	// setup
	defer wg.Done()
	if config.Debug {
		displayChan <- []byte("[BACKUP][START][SERVER] " + server)
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
	// debug: fmt.Printf("fetch: %s --- last: %s\n", sum[:], last[:])
	if sum == last {
		if config.Debug {
			displayChan <- []byte("[BACKUP][SERVER][NO-CHANGE] " + server)
		}
		return
	}

	// check xml file into storage
	if err = checkIntoStore(config, server, serverXML, ts, sum); err != nil {
		displayChan <- []byte("[BACKUP][ERROR][FAIL:XML-STORE-CHECKIN] " + err.Error())
		return
	}
	displayChan <- []byte("[BACKUP][OK][SUCCESS:XML-STORE-CHECKIN-OF-MODIFIED-XML]")
}
