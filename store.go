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
	_ext        = ".xml"
	_archive    = ".archive"
	_tab        = "	"
	_linefeed   = "\n"
	_current    = "current"
	_last       = "last"
	_currentXML = "current.xml"
	_hashFile   = "sha256.db"
)

// lastSum check last XML file sha256 checksum
func lastSum(config *OPNCall, server string) [32]byte {
	fileName := filepath.Join(config.Path, server, _current)
	data, _ := ioutil.ReadFile(fileName)
	return sha256.Sum256(data)
}

// checkIntoStore the XML file
func checkIntoStore(config *OPNCall, server string, serverXML []byte, ts time.Time, sum [32]byte) error {

	// prep storage
	year, month, _ := ts.Date()

	// create store structure
	store := filepath.Join(_archive, strconv.Itoa(year), padMonth(strconv.Itoa(int(month))))
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

	// remove pre-existing last symlink (if any exist)
	_ = os.Remove(_currentXML)

	// write server XML file(s)
	name := ts.UTC().Format("20060102T150405Z") + "-" + server + _ext
	archiveFile := filepath.Join(store, name)
	if err := os.WriteFile(_currentXML, serverXML, 0660); err != nil {
		displayChan <- []byte("[BACKUP][ERROR][FAIL:UNABLE-TO-CREATE-CURRENTFILE] " + server)
		return err
	}
	if err := os.WriteFile(archiveFile, serverXML, 0660); err != nil {
		displayChan <- []byte("[BACKUP][ERROR][FAIL:UNABLE-TO-CREATE-FILE] " + archiveFile)
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

	// remove pre-existing last symlink (if any exist)
	_ = os.Remove(_last)

	// rename current link pointer to last (if any exist)
	_ = os.Rename(_current, _last)

	// write current symlink pointer
	if err = os.Symlink(archiveFile, _current); err != nil {
		displayChan <- []byte("[BACKUP][ERROR][FAIL:UNABLE-TO-CREATE-ARCHIVE-SYMLINK] " + server)
		return err
	}
	return nil
}
