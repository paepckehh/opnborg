package opncentral

import (
	"time"
)

const (
	_dnsSrv          = "127.0.0.1:53"
	_dnsTimeout      = time.Second * 4
	_userAgentPrefix = "opncentral"
)

type OPNCall struct {
	Targets     string
	Key         string
	Secret      string
	NoSSLVerify bool
}
