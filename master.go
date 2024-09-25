package opnborg

import (
	"encoding/xml"
	"errors"
	"strings"
)

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
	}
	if config.Debug {
		displayChan <- []byte("[MASTER][PLUGINS]" + opn.System.Firmware.Plugins)
	}
	config.Sync.PKG.Packages = strings.Split(opn.System.Firmware.Plugins, ",")

	// fin
	if config.Debug {
		displayChan <- []byte("[MASTER][OK][SUCCESS:MASTER-CONFIG-READ-AND-PROCESSED]")
	}
	return config, nil
}
