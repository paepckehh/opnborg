package opnborg

const (
	_empty        = ""
	_userAgent    = "opnborg"
	_apiBackupXML = "/api/core/backup/download/this" // no support for legacy api endpoints
)

type OPNCall struct {
	Targets   string // list of OPNSense Appliances, csv comma seperated
	Key       string // OPNSense Backup User API Key
	Secret    string // OPNSense Backup User API Secret
	TLSKeyPin string // TLS Connection Server Certificate KeyPIN
	AppName   string // Display and SysLog Application Name
	Daemon    bool   // do not daemonize by default (run in background once every hour, log to syslog)
	SSL       bool   // do not verify SSL trustchain against system SSL Trust store, use TLSKeyPIN
	Git       bool   // create and commit all xml files & changes to local .git repo
	Log       bool   // if true, write to syslog (daemon mode) instead to stdout
}
