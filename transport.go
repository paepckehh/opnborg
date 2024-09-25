package opnborg

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	_empty         = ""
	_userAgent     = "opnborg"
	_apiBackupXML  = "/api/core/backup/download/this" // no support for legacy backup api endpoints
	_apiInstallPKG = "/api/core/firmware/install/"    // install packages
)

// installPKG
func installPKG(config *OPNCall, server, pkg string) error {

	// parse & assemble target url
	targetURL := "https://" + server + _apiInstallPKG + pkg
	if _, err := url.Parse(targetURL); err != nil {
		return errors.New("[INSTALL-PKG][UNABLE-TO-PARSE-TARGET-URL]" + targetURL + " " + err.Error())
	}

	// build payload
	params := url.Values{}
	params.Add("", ``)
	post := strings.NewReader(params.Encode())

	// setup request
	req, err := http.NewRequest("POST", targetURL, post)
	if err != nil {
		return errors.New("[INSTALL-PKG][UNABLE-TO-CREATE-HTTP-REQUEST]" + targetURL + " " + err.Error())
	}
	req.Header.Set("User-Agent", _userAgent)
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(config.Key, config.Secret)

	// setup transport layer
	tlsconf := getTlsConf(config)
	transport := getTransport(tlsconf)
	client := getClient(transport)

	// connect
	client.Timeout = time.Duration(4 * time.Second)
	body, err := client.Do(req)
	if err != nil {
		displayChan <- []byte("[INSTALL-PKG][FAIL:TLS-CONNECT] " + targetURL)
		displayChan <- []byte("[INSTALL-PKG][FAIL:TLS-CONNECT] " + err.Error())
		return errors.New("[UNABLE-TO-TLS-CONNECT-SERVER]")
	}

	// read body
	defer body.Body.Close()
	msg, err := io.ReadAll(body.Body)
	if err != nil {
		displayChan <- []byte("[INSTALL-PKG][FAIL:READ-BODY] " + targetURL)
		displayChan <- []byte("[INSTALL-PKG][FAIL:READ-BODY][ERROR] " + err.Error())
		return errors.New("ERROR-WHILE-READ-ANSWER-BODY]" + string(msg))
	}
	if body.StatusCode > 299 {
		displayChan <- []byte("[INSTALL-PKG][FAIL:READ-BODY] " + targetURL)
		displayChan <- []byte("[INSTALL-PKG][FAIL:READ-BODY] " + string(msg))
		return errors.New("ERROR-WHILE-READ-ANSWER-BODY]")
	}
	if config.Debug {
		displayChan <- []byte("[INSTALL-PKG][OK][FINISH] " + targetURL + " -> " + string(msg))
	}
	time.Sleep(12 * time.Second) // wait for action to finish
	return nil
}

// fetchXML file from target server
func fetchXML(server string, config *OPNCall) (data []byte, err error) {

	// parse & assemble target url
	targetURL := "https://" + server + _apiBackupXML
	if _, err = url.Parse(targetURL); err != nil {
		displayChan <- []byte("[FETCH][FAIL:UNABLE-TO-PARSE-TARGET-URL] " + targetURL)
		return nil, errors.New("[UNABLE-TO-PARSE-TARGET-URL]")
	}

	// setup request
	req, err := getRequest(targetURL, _userAgent)
	if err != nil {
		displayChan <- []byte("[FETCH][FAIL:SETUP-URL] " + targetURL)
		return nil, errors.New("[UNABLE-TO-SETUP-TARGET-URL]")
	}
	req.SetBasicAuth(config.Key, config.Secret)

	// setup transport layer
	tlsconf := getTlsConf(config)
	transport := getTransport(tlsconf)
	client := getClient(transport)

	// connect
	client.Timeout = time.Duration(4 * time.Second)
	body, err := client.Do(req)
	if err != nil {
		displayChan <- []byte("[FETCH][FAIL:TLS-CONNECT] " + targetURL)
		displayChan <- []byte("[FETCH][FAIL:TLS-CONNECT] " + err.Error())
		return nil, errors.New("[UNABLE-TO-TLS-CONNECT-SERVER]")
	}

	// read, validate & return full xml body
	defer body.Body.Close()
	data, err = io.ReadAll(body.Body)
	if err != nil {
		displayChan <- []byte("[FETCH][FAIL:READ-BODY] " + targetURL)
		displayChan <- []byte("[FETCH][FAIL:READ-BODY][ERROR] " + err.Error())
		return nil, errors.New("[UNABLE-TO-READ-XML-BODY]")
	}
	if config.Debug {
		displayChan <- []byte("[FETCH][OK][SUCCESS:FETCH] " + targetURL)
	}
	if isValidXML(string(data)) {
		if config.Debug {
			displayChan <- []byte("[FETCH][OK][SUCCESS:XML-VALIDATION] " + targetURL)
		}
		return data, nil
	}
	displayChan <- []byte("[FETCH][ERROR][FAIL:XML-VALIDATION] " + targetURL)
	return nil, errors.New("[INVALID-XML-FILE]")
}

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
