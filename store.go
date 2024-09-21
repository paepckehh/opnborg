package opnborg

import (
	"crypto/sha256"
	"encoding/base64"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// const
const (
	_ext      = ".xml"
	_tab      = "	"
	_linefeed = "\n"
	_latest   = "latest"
	_hashFile = "sha256.db"
)

// lastSum check last XML file sha256 checksum
func lastSum(config *OPNCall, server string) [32]byte {
	fileName := filepath.Join(config.Path, server, _latest)
	data, _ := ioutil.ReadFile(fileName)
	return sha256.Sum256(data)
}

// checkIntoStore the XML file
func checkIntoStore(config *OPNCall, server string, serverXML []byte, ts time.Time, sum [32]byte) error {

	// prep storage
	year, month, _ := ts.Date()

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
	name := ts.UTC().Format("20060102T150405Z") + "-" + server + _ext
	file := filepath.Join(store, name)
	if err := os.WriteFile(file, serverXML, 0660); err != nil {
		displayChan <- []byte("[BACKUP][ERROR][FAIL:UNABLE-TO-CREATE-FILE] " + file)
		return err
	}

	// write hashsum log entry
	logEntry := name + _tab + base64.StdEncoding.EncodeToString(sum[:]) + _linefeed
	hashFile, err := os.OpenFile(_hashFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		displayChan <- []byte("[BACKUP][ERROR][FAIL:UNABLE-TO-OPEN-OR-CREATE-HASHSHUM-FILE] " + server)
		return err
	}
	if _, err := hashFile.Write([]byte(logEntry)); err != nil {
		displayChan <- []byte("[BACKUP][ERROR][FAIL:UNABLE-TO-WRITE-TO-HASHSHUM-FILE] " + server)
		return err
	}
	if err := hashFile.Close(); err != nil {
		displayChan <- []byte("[BACKUP][ERROR][FAIL:UNABLE-TO-SAVE-HASHSHUM-FILE] " + server)
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
