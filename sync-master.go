package opnborg

import (
	"encoding/xml"
	"errors"
	"strings"
	"sync"
)

// global
var (
	syncPKG      string
	syncPKGMutex sync.RWMutex
)

// setSyncPKG stores the master plugin list under syncPKGMutex so the
// HTTP handler reading it (getPKG) never races with the daemon writer.
func setSyncPKG(plugins string) {
	syncPKGMutex.Lock()
	syncPKG = plugins
	syncPKGMutex.Unlock()
}

// getSyncPKG returns a snapshot of the master plugin list.
func getSyncPKG() string {
	syncPKGMutex.RLock()
	defer syncPKGMutex.RUnlock()
	return syncPKG
}

// splitPlugins splits a comma-separated plugin list, trimming the whitespace
// around each entry and skipping empties (the strings.Split("", ",") ==
// [""] gotcha and malformed lists like "os-a, ,os-b" never yield an empty
// package name checkInstallPKG would try to install).
func splitPlugins(plugins string) []string {
	var out []string
	for p := range strings.SplitSeq(plugins, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// readMasterConf
func readMasterConf(config *OPNCall) (*OPNCall, error) {

	// setup
	if config.Debug {
		displayChan <- []byte("[STARTING][MASTER][READ-MASTER-CONFIG]")
	}

	// fetch current XML from master server
	masterXML, err := fetchXML(config.Sync.Master, config)
	if err != nil {
		displayChan <- []byte("[MASTER][ERROR][FAIL:UNABLE-TO-FETCH] " + config.Sync.Master)
		return config, err
	}
	// validate XML
	if !isValidXML(string(masterXML)) {
		return config, errors.New("[INVALID-XML-FILE]")
	}
	if config.Debug {
		displayChan <- []byte("[MASTER][OK][SUCCESS:XML-VALIDATION] " + config.Sync.Master)
	}

	// xml unmarshal
	var opn Opnsense
	if err = xml.Unmarshal(masterXML, &opn); err != nil {
		displayChan <- []byte("[MASTER][ERROR][XML-PARSE][PLUGINS]" + err.Error())
		return config, err
	}
	if config.Debug {
		displayChan <- []byte("[MASTER][PLUGINS]" + opn.System.Firmware.Plugins)
	}
	setSyncPKG(opn.System.Firmware.Plugins) // global, guarded
	config.Sync.PKG.Packages = splitPlugins(opn.System.Firmware.Plugins)

	// fin
	if config.Debug {
		displayChan <- []byte("[MASTER][OK][SUCCESS:MASTER-CONFIG-READ-AND-PROCESSED]")
	}
	return config, nil
}
