package opnborg

const (
	_empty     = ""
	_userAgent = "opnborg"
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
