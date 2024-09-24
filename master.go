package opnborg

// readMasterConf
func readMasterConf(config *OPNCall) *OPNCall {

	// setup
	if config.Debug {
		displayChan <- []byte("[STARTING][MASTER][READ-MASTER-CONFIG]")
	}

	// fetch current XML from master server
	_, err := fetchXML(config.Master, config)
	if err != nil {
		displayChan <- []byte("[MASTER][ERROR][FAIL:UNABLE-TO-FETCH] " + config.Master)
		displayChan <- []byte("[MASTER][ERROR][FAIL:UNABLE-TO-FETCH] " + err.Error())
		return config
	}

	//
	if config.Debug {
		displayChan <- []byte("[Master][OK][SUCCESS:MASTER-CONFIG-READ-AND-PROCESSED]")
	}
	return config
}
