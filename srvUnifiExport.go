package opnborg

// unfi Asset Inventory Export Server
func srvUnifiExport(config *OPNCall) {

	// info
	displayChan <- []byte("[UNIFI][EXPORT][SERVER][START][MONGODB-URI] " + config.Unifi.Export.URI.String())

	// loop forever
	for {

		displayChan <- []byte("[UNIFI][EXPORT][START]")

		displayChan <- []byte("[UNIFI][EXPORT][END]")

		// wait for next round trigger
		<-updateUnifiExport
	}
}
