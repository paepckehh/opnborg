package opnborg

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"os"
)

// getHTTPTLS provides the tcp listener with an hardened tls configuration
func getHTTPTLS(config *OPNCall) (listen net.Listener, err error) {

	// return plain text listener when not CAcert
	if config.CAcert != "" && config.CAkey != "" {

		// read cert & key from file
		key, err := tls.LoadX509KeyPair(config.CAcert, config.CAkey)
		if err != nil {
			return listen, err
		}

		// create cert pool
		caClient := x509.NewCertPool()
		clientAuthMode := tls.VerifyClientCertIfGiven
		if config.CAclient != "" {
			cert, err := os.ReadFile(config.CAclient)
			if err != nil {
				return listen, err
			}
			caClient.AppendCertsFromPEM(cert)
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
			Renegotiation:          0,
		}
		return tls.Listen("tcp", config.ListenAddr, tlsConf)
	}
	return net.Listen("tcp", config.ListenAddr)
}
