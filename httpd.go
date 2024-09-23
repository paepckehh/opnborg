package opnborg

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"os"
)

// get https listener
func getHTTPTLS(config *OPNCall) (listen net.Listener, err error) {

	// return plain text listener when not CAcert
	if config.CAcert != "" && config.CAkey != "" {

		// read cert & key from file
		key, err := tls.LoadX509KeyPair(c.CAcert, c.CAkey)
		if err != nil {
			return listen, err
		}

		// create cert pool
		caClient := x509.NewCertPool()
		clientAuthMode := tls.VerifyClientCertIfGiven
		if config.CAclient != "" {
			cert, err := os.ReadFile(c.CAclient)
			if err != nil {
				return listen, err
			}
			caClient.AppendCertsFromPEM(cert)
			clientAuthMode = tls.RequireAndVerifyClientCert
		}

		// setup hardened tls13-chachapoly1305-only https listener
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
			Renegotiation:          0,
		}
		return tls.Listen("tcp", c.ListenAddr, tlsConf)
	}
	return net.Listen("tcp", c.ListenAddr)
}
