package opnborg

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cnaude/go-syslog/syslog/v3"
)

// httpd spinup the internal rsyslog server
func startRSysLog(config *OPNCall) {

	// terminate if not in daemon mode
	if !config.Daemon || !config.RSysLog {
		displayChan <- []byte("[RSYSLOG][TERMINATED)")
		return
	}

	// create store structure
	logStore := filepath.Join(config.Path, "Logs")
	if err := os.MkdirAll(logStore, 0770); err != nil {
		displayChan <- []byte("[RSYSLOG][ERROR][FAIL:UNABLE-TO-CREATE-FILE-STORAGE] " + logStore + ":" + err.Error())
		return
	}

	// setup
	channel := make(syslog.LogPartsChannel)
	handler := syslog.NewChannelHandler(channel)

	server := syslog.NewServer()
	server.SetFormat(syslog.RFC5424)
	server.SetHandler(handler)
	server.ListenUDP("0.0.0.0:5140")
	server.Boot()

	go func(channel syslog.LogPartsChannel) {
		for logParts := range channel {
			fmt.Println(logParts)
		}
	}(channel)

	// info
	if config.Debug {
		displayChan <- []byte("[RSYSLOG][SPIN-UP-SERVER]")
	}

	server.Wait()
}
