package opnborg

import (
	"os"
	"path/filepath"

	logConfig "paepcke.de/opnborg/tinysyslog/config"
	logServer "paepcke.de/opnborg/tinysyslog/server"
)

// httpd spinup the internal rsyslog server
func startRSysLog(config *OPNCall) {

	// terminate if not in daemon mode
	if !config.Daemon || !config.RSysLog {
		return
	}

	// create store structure
	logStore := filepath.Join(config.Path, "Logs")
	if err := os.MkdirAll(fulllogStore, 0770); err != nil {
		displayChan <- []byte("[RSYSLOG][ERROR][FAIL:UNABLE-TO-CREATE-FILE-STORAGE] " + logStore + ":" + err.Error())
		return
	}

	// setup
	c := logConfig.New()
	c.FilesystemSink.FilesystemSink{
		Filename: logStore,
		MaxAge:   180, // days
		MaxSize:  100, // megabyte
	}

	// serv
	srv, err := logServer.New()
	if err != nil {
		displayChan <- []byte("[RSYSLOG][ERROR][FATAL] " + err.Error())
		return
	}
	srv.Run()
}
