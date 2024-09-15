package opnborg

const (
	_userAgentPrefix = "opnborg"
)

type OPNCall struct {
	Targets     string
	Key         string
	Secret      string
	AppName     string
	NoSSLVerify bool
	Log         bool
}
