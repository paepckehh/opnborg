package opnborg

import (
	"sync"
)

// backupSrv, perform individual server backup
func backupSrv(server string, config *OPNCall, wg *sync.WaitGroup) {
	defer wg.Done()
	displayChan <- []byte("[BACKUP][START][PROCESSING-SERVER] https://" + server + "/api")
}
