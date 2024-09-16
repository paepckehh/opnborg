package opnborg

import (
	"io"
	"sync"
	"time"
)

// backupSrv, perform individual server backup
func backupSrv(server string, config *OPNCall, wg *sync.WaitGroup) {

	// setup
	defer wg.Done()
	displayChan <- []byte("[BACKUP][START][SERVER] " + server)

	// parse & assemble target url
	url := "https://" + server + _apiBackupXML

	// setup request
	req, err := getRequest(url, _userAgent)
	if err != nil {
		displayChan <- []byte("[BACKUP][FAIL:SETUP-URL] " + url)
		return
	}
	req.SetBasicAuth(config.Key, config.Secret)

	// setup transport layer
	tlsconf := getTlsConf(config)
	transport := getTransport(tlsconf)
	client := getClient(transport)

	// setup target slice for xml body
	var data []byte

	// connect
	client.Timeout = time.Duration(4 * time.Second)
	body, err := client.Do(req)
	if err != nil {
		displayChan <- []byte("[BACKUP][FAIL:TLS-CONNECT] " + url)
		displayChan <- []byte("[BACKUP][FAIL:TLS-CONNECT] " + err.Error())
		return
	}

	// read full xml body
	defer body.Body.Close()
	data, err = io.ReadAll(body.Body)
	if err != nil {
		displayChan <- []byte("[BACKUP][FAIL:READ-BODY] " + url)
		displayChan <- []byte("[BACKUP][FAIL:READ-BODY][ERROR] " + err.Error())
		return
	}
	displayChan <- []byte("[BACKUP][OK][SUCCESS:FETCH] " + url)
	if isValidXML(string(data)) {
		displayChan <- []byte("[BACKUP][OK][SUCCESS:XML-VALIDATION] " + url)
		return
	}
	displayChan <- []byte("[BACKUP][ERROR][FAIL:XML-VALIDATION] " + url)
}
