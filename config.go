package opncentral

import (
	"net/url"
	"time"
)

const (
	_dnsSrv          = "127.0.0.1:53"
	_dnsTimeout      = time.Second * 4
	_userAgentPrefix = "opncentral"
)

type OPNCall struct {
	BaseUrl     url.URL
	Key         string
	Secret      string
	UserAgent   string
	NoSSLVerify bool
}
