package opnborg

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
)

// getTlsConf harden tls object settings
func getTlsConf(config *OPNCall) *tls.Config {
	tlsConfig := &tls.Config{
		InsecureSkipVerify:     true,
		SessionTicketsDisabled: true,
		Renegotiation:          0,
		MinVersion:             tls.VersionTLS13,
		MaxVersion:             tls.VersionTLS13,
		CipherSuites:           []uint16{tls.TLS_CHACHA20_POLY1305_SHA256},
		CurvePreferences:       []tls.CurveID{tls.X25519},
	}
	if config.TLSKeyPin != _empty {
		tlsConfig.VerifyConnection = func(state tls.ConnectionState) error {
			if !pinVerifyState(config.TLSKeyPin, &state) {
				return errors.New("keypin verification failed")
			}
			return nil
		}
	}
	return tlsConfig
}

// pinVerifyState verify keypin status
func pinVerifyState(keyPin string, state *tls.ConnectionState) bool {
	if len(state.PeerCertificates) > 0 {
		if keyPin == keyPinBase64(state.PeerCertificates[0]) {
			return true
		}
	}
	return false
}

// keyPinBase64 generate keypin base64
func keyPinBase64(cert *x509.Certificate) string {
	h := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return base64.StdEncoding.EncodeToString(h[:])
}

// getTransport hardened
func getTransport(tlsconf *tls.Config) *http.Transport {
	return &http.Transport{
		Proxy:              http.ProxyFromEnvironment,
		TLSClientConfig:    tlsconf,
		DisableCompression: true, // pre-compressed file downloads
		ForceAttemptHTTP2:  false,
	}
}

// getClient setup hardened transport
func getClient(transport *http.Transport) *http.Client {
	return &http.Client{
		CheckRedirect: nil,
		Jar:           nil,
		Transport:     transport,
	}
}

// getRequest setup hardened http request
func getRequest(targetURL, userAgent string) (*http.Request, error) {
	u, err := url.Parse(targetURL)
	if err != nil {
		return &http.Request{}, err
	}
	return &http.Request{
		URL:        u,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header: http.Header{
			"User-Agent": []string{userAgent},
		},
	}, nil
}
