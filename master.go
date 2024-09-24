package opnborg

import (
	"encoding/xml"
	"errors"
)

// opnsense
type opnsense struct {
	XMLName xml.Name `xml:opnsense`
	system  system
}

// system
type system struct {
	XMLName xml.Name `xml:system`
	plugins string
}

// readMasterConf
func readMasterConf(config *OPNCall) (*OPNCall, error) {

	// setup
	if config.Debug {
		displayChan <- []byte("[STARTING][MASTER][READ-MASTER-CONFIG]")
	}

	// fetch current XML from master server
	masterXML, err := fetchXML(config.Master, config)
	if err != nil {
		displayChan <- []byte("[MASTER][ERROR][FAIL:UNABLE-TO-FETCH] " + config.Master)
		return config, err
	}
	// validate XML
	if isValidXML(string(masterXML)) {
		if config.Debug {
			displayChan <- []byte("[MASTER][OK][SUCCESS:XML-VALIDATION] " + config.Master)
		}
	} else {
		return config, errors.New("[INVALID-XML-FILE]")
	}

	// xml unmarshal
	var opn opnsense
	xml.Unmarshal(masterXML, &opn)
	displayChan <- []byte("[MASTER][PLUGINS]" + opn.system.plugins)

	// fin
	if config.Debug {
		displayChan <- []byte("[MASTER][OK][SUCCESS:MASTER-CONFIG-READ-AND-PROCESSED]")
	}
	return config, nil
}
