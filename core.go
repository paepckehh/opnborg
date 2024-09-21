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
const (
	_ext    = ".xml"
	_latest = "latest"
)

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

	// check xml file into storage
	if err = checkIntoStore(config, server, serverXML, now); err != nil {
		displayChan <- []byte("[BACKUP][ERROR][FAIL:XML-STORE-CHECKIN] " + err.Error())
		return
	}
	displayChan <- []byte("[BACKUP][OK][SUCCESS:XML-STORE-CHECKIN]")
}

// checkIntoStore the XML file
func checkIntoStore(config *OPNCall, server string, serverXML []byte, now time.Time) (err error) {

	// prep storage
	year, month, _ := now.Date()

	// create store structure
	store := filepath.Join(strconv.Itoa(year), padMonth(strconv.Itoa(int(month))))
	fullPath := filepath.Join(config.Path, server, store)
	if err := os.MkdirAll(fullPath, 0770); err != nil {
		displayChan <- []byte("[BACKUP][ERROR][FAIL:UNABLE-TO-CREATE-FILE-STORAGE] " + fullPath)
		return err
	}

	// change thread into store-root (needed for relative symlink creation)
	dirStoreRoot := filepath.Join(config.Path, server)
	if err := os.Chdir(dirStoreRoot); err != nil {
		displayChan <- []byte("[BACKUP][ERROR][FAIL:UNABLE-TO-CHANGE-INTO-STORAGE-DIR] " + dirStoreRoot)
		return err
	}

	// write server XML file
	file := filepath.Join(store, now.Format(time.RFC3339)+"-"+server+_ext)
	if err = os.WriteFile(file, serverXML, 0770); err != nil {
		displayChan <- []byte("[BACKUP][ERROR][FAIL:UNABLE-TO-CREATE-FILE] " + file)
		return err
	}

	// remove pre-existing latest symlink (if any)
	_ = os.Remove(_latest)

	// write latest symlink
	if err = os.Symlink(file, _latest); err != nil {
		displayChan <- []byte("[BACKUP][ERROR][FAIL:UNABLE-TO-CREATE-LATEST-SYMLINK] " + server)
		return err
	}
	return nil
}

// fetchXML file from target server
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
