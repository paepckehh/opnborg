package opnborg

import (
	"encoding/xml"
	"errors"
	"fmt"
)

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
	if !isValidXML(string(masterXML)) {
		return config, errors.New("[INVALID-XML-FILE]")
	}
	if config.Debug {
		displayChan <- []byte("[MASTER][OK][SUCCESS:XML-VALIDATION] " + config.Master)
	}

	// xml unmarshal
	var opn opnsense
	if err = xml.Unmarshal(masterXML, &opn); err != nil {
		displayChan <- []byte("[MASTER][ERROR][XML-PARSE][PLUGINS]" + err.Error())
	}
	displayChan <- []byte("[MASTER][PLUGINS]" + opn.system.plugins)
	fmt.Println(opn)

	// fin
	if config.Debug {
		displayChan <- []byte("[MASTER][OK][SUCCESS:MASTER-CONFIG-READ-AND-PROCESSED]")
	}
	return config, nil
}
