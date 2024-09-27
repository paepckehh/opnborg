package opnborg

import (
	"encoding/xml"
	"errors"
	"strings"
)

// checkRSysLogConfig
func checkRSysLogConfig(server string, config *OPNCall) error {

	// fetch current XML config from server
	masterXML, err := fetchXML(server, config)
	if err != nil {
		displayChan <- []byte("[RSYSLOG][ERROR][FAIL:UNABLE-TO-FETCH] " + server)
		return err
	}

	// validate XML
	if !isValidXML(string(masterXML)) {
		return errors.New("[INVALID-XML-FILE]")
	}
	if config.Debug {
		displayChan <- []byte("[RSYSLOG][OK][SUCCESS:XML-VALIDATION] " + server)
	}

	// xml unmarshal
	var opn Opnsense
	if err = xml.Unmarshal(masterXML, &opn); err != nil {
		displayChan <- []byte("[RSYSLOG][ERROR][XML-PARSE][PLUGINS]" + err.Error())
		return err
	}

	targetSRV := strings.Split(config.RSysLog.Server, ":")

	// compare
	if opn.OPNsense.Syslog.Destinations.Destination.Hostname == targetSRV[0] {
		return nil
	}
	if opn.OPNsense.Syslog.Destinations.Destination.Port == targetSRV[1] {
		return nil
	}
	return errors.New("[RSYSLOG-CONFIG-CHECK][FAIL]" + server)
}
