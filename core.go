package opnborg

import (
	"io"
	"sync"
	"time"
)

// backupSrv, perform individual server backup
func backupSrv(server string, config *OPNCall, wg *sync.WaitGroup) {

	defer wg.Done()
	displayChan <- []byte("[BACKUP][START][SERVER] " + server)

	// parse & assemble target url
	url := "https://" + server + "/api/backup"

	// setup request
	request, err := getRequest(url, _userAgent)
	if err != nil {
		displayChan <- []byte("[BACKUP][FAIL:SETUP-URL][SERVER] " + url)
		return
	}

	// setup transport layer
	tlsconf := getTlsConf(_empty)
	transport := getTransport(tlsconf)
	client := getClient(transport)

	// setup target slice for xml body
	var data []byte

	// connect
	client.Timeout = time.Duration(4 * time.Second)
	request.Method = "GET"
	body, err := client.Do(request)
	if err != nil {
		displayChan <- []byte("[BACKUP][FAIL:CONNECT-SERVER][SERVER] " + url)
		displayChan <- []byte("[BACKUP][FAIL:CONNECT-SERVER][ERROR] " + err.Error())
		return
	}

	// read full xml body
	data, err = io.ReadAll(body.Body)
	if err != nil || body.StatusCode > 299 {
		displayChan <- []byte("[BACKUP][FAIL:READ-BODY][SERVER] " + url)
		displayChan <- []byte("[BACKUP][FAIL:CONNECT-SERVER][ERROR] " + err.Error())
		return
	}
	displayChan <- []byte(data)
	body.Body.Close()
}
