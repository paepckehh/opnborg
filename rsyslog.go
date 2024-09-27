package opnborg

import (
	"os"
	"path/filepath"

	"github.com/cnaude/go-syslog/syslog/v3"
	"github.com/natefinch/lumberjack"
	"github.com/sirupsen/logrus"
)

// httpd spinup the internal rsyslog server
func startRSysLog(config *OPNCall) {

	// terminate if not in daemon mode
	if !config.Daemon || !config.RSysLog.Enable {
		displayChan <- []byte("[RSYSLOG][TERMINATED)")
		return
	}

	// create store structure
	logStore := filepath.Join(config.Path, "Logs")
	if err := os.MkdirAll(logStore, 0770); err != nil {
		displayChan <- []byte("[RSYSLOG][ERROR][FAIL:UNABLE-TO-CREATE-FILE-STORAGE] " + logStore + ":" + err.Error())
		return
	}

	// setup log storage
	logFile := filepath.Join(logStore, "current.log")
	log := logrus.New()
	log.SetFormatter(&logrus.TextFormatter{})
	log.SetReportCaller(false)
	log.SetOutput(&lumberjack.Logger{
		Filename:   logFile,
		MaxSize:    256,  // max log file size in MB
		MaxBackups: 256,  // max number of old log files to keep (results in max 64GB uncompressed log storage)
		MaxAge:     180,  // max age in days to keep a log file
		Compress:   true, // Compress old log files
	})

	// setup log server
	channel := make(syslog.LogPartsChannel)
	handler := syslog.NewChannelHandler(channel)
	server := syslog.NewServer()
	server.SetFormat(syslog.RFC5424)
	server.SetHandler(handler)
	server.ListenUDP(config.RSysLog.Server)
	server.Boot()

	go func(channel syslog.LogPartsChannel) {
		for line := range channel {
			log.Info(line)
		}
	}(channel)

	// info
	if config.Debug {
		displayChan <- []byte("[RSYSLOG][SPIN-UP-LOG-SERVER] listen interface (udp): " + config.RSysLog.Server)
		displayChan <- []byte("[RSYSLOG][SPIN-UP-LOG-SERVER] logging to: " + logFile)
	}

	server.Wait()
}
