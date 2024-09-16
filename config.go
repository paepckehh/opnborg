package opnborg

const (
	_empty        = ""
	_userAgent    = "opnborg"
	_apiBackupXML = "/api/core/backup/download/this" // no support for legacy api endpoints
)

type OPNCall struct {
	Targets   string
	Key       string
	Secret    string
	TLSKeyPin string
	AppName   string
	Git       bool
	Daemon    bool
	SSL       bool
	Log       bool
}
