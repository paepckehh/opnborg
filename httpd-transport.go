package opnborg

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"os"
)

// getHTTPTLS provides the tcp listener with an hardened tls configuration
func getHTTPTLS(config *OPNCall) (listen net.Listener, err error) {

	// refuse a half-configured cert pair instead of silently falling back to
	// plain-text HTTP (an operator who sets only OPN_HTTPD_CACERT expects TLS,
	// and a silent downgrade would leak the whole WebUI unencrypted).
	if (config.Httpd.CAcert != "") != (config.Httpd.CAkey != "") {
		return nil, errors.New("httpd TLS misconfigured: OPN_HTTPD_CACERT and OPN_HTTPD_CAKEY must be set together")
	}

	// return plain text listener when not CAcert
	if config.Httpd.CAcert != "" && config.Httpd.CAkey != "" {

		// read cert & key from file
		key, err := tls.LoadX509KeyPair(config.Httpd.CAcert, config.Httpd.CAkey)
		if err != nil {
			return nil, err
		}

		// create cert pool
		caClient := x509.NewCertPool()
		clientAuthMode := tls.VerifyClientCertIfGiven
		if config.Httpd.CAClient != "" {
			cert, err := os.ReadFile(config.Httpd.CAClient)
			if err != nil {
				return nil, err
			}
			// a client CA file without a single parsable certificate would arm
			// mTLS with an empty pool (every client rejected) with no hint why
			if !caClient.AppendCertsFromPEM(cert) {
				return nil, errors.New("httpd TLS: no valid PEM certificates in " + config.Httpd.CAClient)
			}
			clientAuthMode = tls.RequireAndVerifyClientCert
		}

		// setup hardened tls13-chachapoly1305-only https http1.1 listener
		tlsConf := &tls.Config{
			Certificates:           []tls.Certificate{key},
			ClientCAs:              caClient,
			ClientAuth:             clientAuthMode,
			MinVersion:             tls.VersionTLS13,
			MaxVersion:             tls.VersionTLS13,
			CipherSuites:           []uint16{tls.TLS_CHACHA20_POLY1305_SHA256},
			CurvePreferences:       []tls.CurveID{tls.X25519},
			NextProtos:             []string{"http/1.1"},
			SessionTicketsDisabled: true,
			Renegotiation:          tls.RenegotiateNever,
		}
		return tls.Listen("tcp", config.Httpd.Server, tlsConf)
	}
	return net.Listen("tcp", config.Httpd.Server)
}
