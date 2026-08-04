package opnborg

import (
	"crypto/sha256"
	"time"

	"paepcke.de/uniex"
)

// global
const _uniEx = "unifi-export"

// unfi Asset Inventory Export Server
func srvUnifiExport(config *OPNCall) {

	// info
	displayChan <- []byte("[UNIFI][EXPORT][SERVER][START][MONGODB-URI] " + config.Unifi.Export.URI.String())
	c := &uniex.Config{
		MongoDB: config.Unifi.Export.URI.String(),
		Format:  config.Unifi.Export.Format,
		Scope:   "client",
	}
	// loop forever
	for {
		// start notice
		displayChan <- []byte("[UNIFI][EXPORT][START]")

		// fetch and calc checksum
		data, err := c.Export()
		if err != nil {
			displayChan <- []byte("[UNIFI][EXPORT][FAIL] " + err.Error())
			continue
		}
		sum := sha256.Sum256(data)

		// fetch and check unifi file into storage
		if err = checkIntoStore(config, _uniEx, config.Unifi.Export.Format, data, time.Now(), sum); err != nil {
			displayChan <- []byte("[UNIFI][EXPORT][FAIL:DATA-STORE-CHECKIN] " + err.Error())
		}

		// end notice
		displayChan <- []byte("[UNIFI][EXPORT][END]")

		// wait for next round trigger
		<-updateUnifiExport
	}
}
