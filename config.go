package opnborg

const (
	_userAgentPrefix = "opnborg"
)

type OPNCall struct {
	Targets string
	Key     string
	Secret  string
	TLSpin  string
	AppName string
	Git     bool
	Daemon  bool
	NoSSL   bool
	Log     bool
}
