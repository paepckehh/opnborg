package opnborg

import (
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// const
const _ext = ".xml"

// backupSrv, perform individual server backup
func backupSrv(server string, config *OPNCall, wg *sync.WaitGroup) {

	// setup
	defer wg.Done()
	displayChan <- []byte("[BACKUP][START][SERVER] " + server)

	// connection succeed, set exact backup server timestamp before fetch
	now := time.Now()

	// fetch current XML backup from server
	serverXML, err := fetchXML(server, config)
	if err != nil {
		displayChan <- []byte("[BACKUP][ERROR][FAIL:UNABLE-TO-FETCH] " + server)
		displayChan <- []byte("[BACKUP][ERROR][FAIL:UNABLE-TO-FETCH] " + err.Error())
		return
	}

	// prep local backup file structure
	if config.Path == "" {
		config.Path = filepath.Dir("./")
	}

	// prep timestamps
	year, month, _ := now.Date()

	// create store structure
	dirPath := filepath.Join(config.Path, server, strconv.Itoa(year), padMonth(strconv.Itoa(int(month))))
	if err := os.MkdirAll(dirPath, 0770); err != nil {
		displayChan <- []byte("[BACKUP][ERROR][FAIL:UNABLE-TO-CREATE-FILE-STORAGE] " + dirPath)
		displayChan <- []byte("[BACKUP][ERROR][FAIL:UNABLE-TO-CREATE-FILE-STORAGE] " + err.Error())
		return
	}

	// write server XML file
	fileName := filepath.Join(dirPath, now.Format(time.RFC3339)+"-"+server+_ext)
	if err = os.WriteFile(fileName, serverXML, 0770); err != nil {
		displayChan <- []byte("[BACKUP][ERROR][FAIL:UNABLE-TO-CREATE-FILE] " + fileName)
		displayChan <- []byte("[BACKUP][ERROR][FAIL:UNABLE-TO-CREATE-FILE] " + err.Error())
		return
	}
	displayChan <- []byte("[BACKUP][OK][SUCCESS:WRITE-XML-FILE] " + fileName)
}

// fetchXML file from server
func fetchXML(server string, config *OPNCall) (data []byte, err error) {

	// parse & assemble target url
	targetURL := "https://" + server + _apiBackupXML
	if _, err = url.Parse(targetURL); err != nil {
		displayChan <- []byte("[BACKUP][FAIL:UNABLE-TO-PARSE-TARGET-URL] " + targetURL)
		return nil, errors.New("[UNABLE-TO-PARSE-TARGET-URL]")
	}

	// setup request
	req, err := getRequest(targetURL, _userAgent)
	if err != nil {
		displayChan <- []byte("[BACKUP][FAIL:SETUP-URL] " + targetURL)
		return nil, errors.New("[UNABLE-TO-SETUP-TARGET-URL]")
	}
	req.SetBasicAuth(config.Key, config.Secret)

	// setup transport layer
	tlsconf := getTlsConf(config)
	transport := getTransport(tlsconf)
	client := getClient(transport)

	// connect
	client.Timeout = time.Duration(4 * time.Second)
	body, err := client.Do(req)
	if err != nil {
		displayChan <- []byte("[BACKUP][FAIL:TLS-CONNECT] " + targetURL)
		displayChan <- []byte("[BACKUP][FAIL:TLS-CONNECT] " + err.Error())
		return nil, errors.New("[UNABLE-TO-TLS-CONNECT-SERVER]")
	}

	// read, validate & return full xml body
	defer body.Body.Close()
	data, err = io.ReadAll(body.Body)
	if err != nil {
		displayChan <- []byte("[BACKUP][FAIL:READ-BODY] " + targetURL)
		displayChan <- []byte("[BACKUP][FAIL:READ-BODY][ERROR] " + err.Error())
		return nil, errors.New("[UNABLE-TO-READ-XML-BODY]")
	}
	displayChan <- []byte("[BACKUP][OK][SUCCESS:FETCH] " + targetURL)
	if isValidXML(string(data)) {
		displayChan <- []byte("[BACKUP][OK][SUCCESS:XML-VALIDATION] " + targetURL)
		return data, nil
	}
	displayChan <- []byte("[BACKUP][ERROR][FAIL:XML-VALIDATION] " + targetURL)
	return nil, errors.New("[INVALID-XML-FILE]")
}
