package opnborg

// checkInstallPKG checks the target server for missing packages
func checkInstallPKG(server string, config *OPNCall) error {

	// fetch current XML config from server
	_, err := fetchXML(server, config)
	if err != nil {
		displayChan <- []byte("[SYNC][ERROR][FAIL:UNABLE-TO-FETCH] " + server)
		return err
	}
	return nil
}
