package opnborg

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"time"
)

// perform unifi backup
func srvUnifiBackup(config *OPNCall) {

	// setup
	server, notice := config.Unifi.WebUI.Hostname(), ""
	displayChan <- []byte("[UNIFI][BACKUP][START][CONTROLLER] " + server)

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
	system := map[string]any{"cmd": "async-backup", "days": 0}
	postSystem, err := json.Marshal(system)
	if err != nil {
		displayChan <- []byte("[UNIFI][BACKUP][ERROR][CONFIG-SYSTEM-TEST-JSON-ENCODING-FAIL]" + err.Error())
		return // unrecoverable
	}

	// init
	ts := time.Now()
	isReachable, backupOK, notice := false, false, ""

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
				body, _ := io.ReadAll(res.Body)
				_ = res.Body.Close()
				notice = "[UNIFI][BACKUP][ERROR][UNABLE-TO-AUTENTHICATE][BODY] "
				displayChan <- []byte(notice)
				displayChan <- body
			} else {
				// success: drain and close the login response before reusing res
				_, _ = io.Copy(io.Discard, res.Body)
				_ = res.Body.Close()
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
						body, _ := io.ReadAll(res.Body)
						_ = res.Body.Close()
						displayChan <- []byte(notice)
						displayChan <- body

					} else {
						// success: drain and close the system response before the download GET reuses res
						_, _ = io.Copy(io.Discard, res.Body)
						_ = res.Body.Close()
					}
				}
			}
		}

		// if reachable, proceed with backup
		if isReachable {

			// if last backup > 6 hours
			if time.Since(ts) > 6*time.Hour {
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

				// proceed
				if backupOK {

					// read body
					unf, err := io.ReadAll(res.Body)
					if err != nil {
						backupOK = false
						notice = "[UNIFI][BACKUP][ERROR][BACKUP-DOWNLOAD-FILE-BODY-FAIL] " + err.Error()
						displayChan <- []byte(notice)
					}
					_ = res.Body.Close()

					// check file
					if backupOK {
						if len(unf) < 1024 {
							backupOK = false
							notice = "[UNIFI][BACKUP][ERROR][BACKUP-DOWNLOAD-FILE-TO-SMALL]"
							displayChan <- []byte(notice)
						}

						// check into store
						if backupOK {
							if err := checkIntoStore(config, config.Unifi.WebUI.Hostname(), "unf", unf, ts, sha256.Sum256(unf)); err != nil {
								backupOK = false
								notice = "[UNIFI][BACKUP][ERROR][UNABLE-TO-WRITE-BACKUP-FILE-INTO-STORE] " + err.Error()
								displayChan <- []byte(notice)
							} else {
								// flag git store as dirty only on a successful checkin
								config.dirty.Store(true)
								displayChan <- []byte("[UNIFI][BACKUP][SUCCESSFUL]")
							}
						}
					}
				}
				displayChan <- []byte("[UNIFI][BACKUP][END]")
			}
		}

		// set unifi status
		setUnifiStatus(config, server, config.Unifi.Tag, notice, time.Now(), isReachable, backupOK)

		// wait for next round trigger
		<-updateUnifiBackup
	}
}
