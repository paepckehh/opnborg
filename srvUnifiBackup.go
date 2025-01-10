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
func srvUnifiBackup(config *OPNCall) {

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

	// init
	ts := time.Now()
	isReachable, backupOK, notice := true, false, ""

	// enfore init backup
	unifiBackupNow.Store(true)

	// loop forever
	for {
		// reset default state
		isReachable, backupOK, notice = true, false, "status:ok"

		// perform authentication
		res, err := client.Post(config.Unifi.WebUI.String()+"/api/login", "application/json", bytes.NewBuffer(postLogin))
		if err != nil {
			isReachable = false
			notice = "[UNIFI][BACKUP][ERROR][UNABLE-TO-AUTENTHICATE]" + err.Error()
			displayChan <- []byte(notice)
		}

		// was authentication ok?
		if isReachable {

			// check http status code
			if res.StatusCode != 200 {
				isReachable = false
				body, _ := ioutil.ReadAll(res.Body)
				notice = "[UNIFI][BACKUP][ERROR][UNABLE-TO-AUTENTHICATE][BODY] "
				displayChan <- []byte(notice)
				displayChan <- body
			}

			// was authentication and status code ok?
			if isReachable {

				// perform actual fetch test
				res, err = client.Post(config.Unifi.WebUI.String()+"/api/s/default/cmd/system", "application/json", bytes.NewBuffer(postSystem))
				if err != nil {
					isReachable = false
					notice = "[UNIFI][BACKUP][ERROR][CONFIG-DOWNLOAD-FAIL] " + err.Error()
					displayChan <- []byte(notice)
				}
				if isReachable {
					// was fetch sucessfull, check http code
					if res.StatusCode != 200 {
						isReachable = false
						notice = "[UNIFI][BACKUP][ERROR][CONFIG-DOWNLOAD-FAIL][BODY] "
						body, _ := ioutil.ReadAll(res.Body)
						displayChan <- []byte(notice)
						displayChan <- body

					}
				}
			}
		}

		// if reachable, proceed with backup
		if isReachable {

			// if last backup > 6 hours
			if time.Now().Sub(ts) < time.Duration(6*time.Hour) {
				unifiBackupNow.Store(true)
			}

			// perform backup
			if unifiBackupNow.Load() {

				// reset unifiBackupNow
				unifiBackupNow.Store(false)

				// update timestamp
				ts = time.Now()

				// setup
				backupOK = true

				// download backup file
				res, err = client.Get(config.Unifi.WebUI.String() + "/dl/backup/" + config.Unifi.Version + ".unf")
				if err != nil {
					backupOK = false
					notice = "[UNIFI][BACKUP][ERROR][BACKUP-DOWNLOAD-FILE-HEAD-FAIL] " + err.Error()
					displayChan <- []byte(notice)
				}
				defer res.Body.Close()

				// proceed
				if backupOK {

					// read body
					unf, err := io.ReadAll(res.Body)
					if err != nil {
						backupOK = false
						notice = "[UNIFI][BACKUP][ERROR][BACKUP-DOWNLOAD-FILE-BODY-FAIL] " + err.Error()
						displayChan <- []byte(notice)
					}

					// check file
					if backupOK {
						if len(unf) < 1024 {
							backupOK = false
							notice = "[UNIFI][BACKUP][ERROR][BACKUP-DOWNLOAD-FILE-TO-SMALL] " + err.Error()
							displayChan <- []byte(notice)
						}

						// check into store
						if backupOK {

							// check into store
							checkIntoStore(config, config.Unifi.WebUI.Hostname(), "unf", unf, ts, sha256.Sum256(unf))

							// flag git store as dirty
							config.dirty.Store(true)

							// notify
							displayChan <- []byte("[UNIFI][BACKUP][SUCCESSFUL]")

						}
					}
				}
				displayChan <- []byte("[UNIFI][BACKUP][END]")
			}
		}

		// set unifi status
		setUnifiStatus(config, time.Now(), notice, isReachable, backupOK)

		// wait for next round trigger
		<-updateUnifiBackup
	}
}
