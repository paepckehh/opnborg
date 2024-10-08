package opnborg

import (
	"strings"
)

// checkInstallPKG checks the target server for missing packages
func checkInstallPKG(server string, config *OPNCall, opn *Opnsense) error {

	// extract
	srvpkg := strings.Split(opn.System.Firmware.Plugins, ",")

	// compare
	var add bool
	var missing []string
	for _, master := range config.Sync.PKG.Packages {
		add = true
		for _, pkg := range srvpkg {
			if master == pkg {
				add = false
				break
			}
		}
		if add {
			missing = append(missing, master)
		}
	}
	if len(missing) > 0 {
		displayChan <- []byte("[SYNC][MISSING-PKG]" + server + ":" + strings.Join(missing, ","))
	}

	// install missing pkg
	for _, pkg := range missing {
		if err := installPKG(config, server, pkg); err != nil {
			displayChan <- []byte("[SYNC][PKG][FAIL][INSTALL]" + pkg + " -> " + server)
		} else {
			if config.Debug {
				displayChan <- []byte("[SYNC][PKG][DONE]" + pkg + " -> " + server)
			}
		}
	}

	// fin
	if config.Debug {
		displayChan <- []byte("[SYNC][FINISH]" + server)
	}

	return nil
}
