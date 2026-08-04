package opnborg

import (
	"fmt"
	"slices"
	"strings"
)

// checkInstallPKG checks the target server for missing packages and installs
// them. A failure to install any package is aggregated and returned as an
// error so the caller (actionOPN) can mark the hive member as degraded.
func checkInstallPKG(server string, config *OPNCall, opn *Opnsense) error {

	// extract installed plugins from the target host (empty => none installed)
	srvpkg := splitPlugins(opn.System.Firmware.Plugins)

	// compare against the master package list
	var missing []string
	for _, master := range config.Sync.PKG.Packages {
		if !slices.Contains(srvpkg, master) {
			missing = append(missing, master)
		}
	}
	if len(missing) > 0 {
		displayChan <- []byte("[SYNC][MISSING-PKG]" + server + ":" + strings.Join(missing, ","))
	}

	// install missing pkg
	var failed []string
	for _, pkg := range missing {
		if err := installPKG(config, server, pkg); err != nil {
			displayChan <- []byte("[SYNC][PKG][FAIL][INSTALL]" + pkg + " -> " + server)
			failed = append(failed, pkg)
			continue
		}
		if config.Debug {
			displayChan <- []byte("[SYNC][PKG][DONE]" + pkg + " -> " + server)
		}
	}

	// fin
	if config.Debug {
		displayChan <- []byte("[SYNC][FINISH]" + server)
	}

	// aggregate install failures so the caller can flag the host degraded
	if len(failed) > 0 {
		return fmt.Errorf("[SYNC][PKG][INSTALL-FAIL] %s: %s", server, strings.Join(failed, ","))
	}
	return nil
}
