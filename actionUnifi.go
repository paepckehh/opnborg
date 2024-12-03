package opnborg

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"time"
)

// perform unifi backup
func unifiBackupServer(config *OPNCall) {

	// info
	displayChan <- []byte("[UNIFI][BACKUP][START][CONTROLLER] " + config.Unifi.WebUI.Hostname())

	// setup session
	jar, err := cookiejar.New(nil)
	if err != nil {
		displayChan <- []byte("[UNIFI][BACKUP][ERROR][UNABLE-TO-SETUP-COOKIE-JAR]" + err.Error())
		return // unrecoverable
	}

	// setup tls secure transport
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	client := http.Client{
		Jar:       jar,
		Transport: transport,
	}

	// prep login
	login := map[string]string{"username": config.Unifi.Backup.User, "password": config.Unifi.Backup.Secret}
	postLogin, err := json.Marshal(login)
	if err != nil {
		displayChan <- []byte("[UNIFI][BACKUP][ERROR][CREDENTIALS-JSON-ENCODING-FAIL]" + err.Error())
		return // unrecoverable
	}

	// prep system test
	system := map[string]interface{}{"cmd": "async-backup", "days": 0}
	postSystem, err := json.Marshal(system)
	if err != nil {
		displayChan <- []byte("[UNIFI][BACKUP][ERROR][CONFIG-SYSTEM-TEST-JSON-ENCODING-FAIL]" + err.Error())
		return // unrecoverable
	}

	// setup
	reachable := true

	// perform actual login
	res, err := client.Post(config.Unifi.WebUI.String()+"/api/login", "application/json", bytes.NewBuffer(postLogin))
	if err != nil {
		reachable = false
		displayChan <- []byte("[UNIFI][BACKUP][ERROR][UNABLE-TO-AUTENTHICATE]" + err.Error())
	}
	if res.StatusCode != 200 {
		reachable = false
		body, _ := ioutil.ReadAll(res.Body)
		displayChan <- []byte("[UNIFI][BACKUP][ERROR][UNABLE-TO-AUTENTHICATE][BODY] ")
		displayChan <- body
	}

	// perform actual system reachable test
	res, err = client.Post(config.Unifi.WebUI.String()+"/api/s/default/cmd/system", "application/json", bytes.NewBuffer(postSystem))
	if err != nil {
		reachable = false
		displayChan <- []byte("[UNIFI][BACKUP][ERROR][CONFIG-DOWNLOAD-FAIL] " + err.Error())
	}
	if res.StatusCode != 200 {
		reachable = false
		body, _ := ioutil.ReadAll(res.Body)
		displayChan <- []byte("[UNIFI][BACKUP][ERROR][CONFIG-DOWNLOAD-FAIL][BODY] ")
		displayChan <- body
	}

	// perform backup
	ts := time.Now()
	if reachable {

		// download backup file
		res, err = client.Get(config.Unifi.WebUI.String() + "/dl/backup/" + config.Unifi.Version + ".unf")
		if err != nil {
			displayChan <- []byte("[UNIFI][BACKUP][ERROR][BACKUP-DOWNLOAD-FILE-HEAD-FAIL] " + err.Error())
		}
		defer res.Body.Close()

		// write file
		if err == nil {

			// read body
			unf, err := io.ReadAll(res.Body)
			if err != nil {
				displayChan <- []byte("[UNIFI][BACKUP][ERROR][BACKUP-DOWNLOAD-FILE-BODY-FAIL] " + err.Error())
			}

			// check into store
			if err == nil {
				checkIntoStore(config, "unifi-"+config.Unifi.WebUI.Hostname(), "unf", unf, ts, sha256.Sum256(unf))
				displayChan <- []byte("[UNIFI][BACKUP][SUCCESSFUL]")

				// flag git store as dirty
				config.dirty.Store(true)
			}
		}
		displayChan <- []byte("[UNIFI][BACKUP][FINISH]")
	}
}
