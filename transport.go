package opnborg

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	_empty              = ""
	_userAgent          = "opnborg"
	_apiBackupXML       = "/api/core/backup/download/this" // no support for legacy backup api endpoints
	_apiInstallPKG      = "/api/core/firmware/install/"    // install packages
	_apiFirmwareVersion = "/api/core/firmware/status/"     // firmware version

	// Backup XML fetch retry policy: the download is a plain idempotent GET,
	// so a transient response-framing failure (a truncated body, surplus bytes
	// on the TLS record stream, a mid-body network reset) is retried on a
	// FRESH TCP+TLS connection instead of failing the whole backup pass for
	// that appliance. The observed "Unsolicited response received on idle
	// HTTP channel starting with ..." + "[FAIL:XML-VALIDATION]" pair is a
	// FreeBSD kTLS/nginx record-stream split: the transport delivers exactly
	// the Content-Length-declared prefix (an XML document cut in half), the
	// surplus tail then arrives as unsolicited data on the pooled connection.
	// A fresh connection per attempt dodges the broken stream deterministically.
	_fetchMaxAttempts = 3               // total fetch attempts per XML download
	_fetchRetrySleep  = 1 * time.Second // linear backoff between attempts
	_fetchTimeoutSec  = 20              // per-attempt client timeout (seconds)
)

// fetchError wraps a failed fetch attempt with its retry verdict: transient
// transport/framing failures are retried on a fresh connection, definitive
// API rejections (auth, routing, config errors) are not.
type fetchError struct {
	err       error
	retryable bool
}

func (e *fetchError) Error() string { return e.err.Error() }
func (e *fetchError) Unwrap() error { return e.err }

// fetchRetryable reports whether a failed fetch attempt is worth a retry.
func fetchRetryable(err error) bool {
	var fe *fetchError
	return errors.As(err, &fe) && fe.retryable
}

// getFirmwareVersion
func getFirmwareVersion(config *OPNCall, server string) string {

	// setup
	var err error

	// parse & assemble target url
	targetURL := "https://" + server + _apiFirmwareVersion
	if _, err = url.Parse(targetURL); err != nil {
		displayChan <- []byte("[FETCH-VERSION][FAIL:UNABLE-TO-PARSE-TARGET-URL] " + targetURL + " " + err.Error())
		return "fail"
	}

	// setup request
	req, err := getRequest(targetURL, _userAgent)
	if err != nil {
		displayChan <- []byte("[FETCH-VERSION][FAIL:SETUP-URL] " + targetURL + " " + err.Error())
		return "fail"
	}
	req.SetBasicAuth(config.Key, config.Secret)

	// connect
	client := opnClient(config, 20)
	body, err := client.Do(req)
	if err != nil {
		displayChan <- []byte("[FETCH-VERSION][FAIL:TLS-CONNECT] " + targetURL + " " + err.Error())
		return "fail"
	}

	// read, validate & return full xml body
	defer func() { _ = body.Body.Close() }()
	data, err := io.ReadAll(body.Body)
	if err != nil {
		displayChan <- []byte("[FETCH-VERSION][FAIL:READ-BODY] " + targetURL + err.Error())
		return "fail"
	}

	// a non-2xx answer is an auth or api failure page, never valid JSON;
	// surface the status instead of a misleading JSON-parse failure.
	if body.StatusCode < 200 || body.StatusCode > 299 {
		displayChan <- []byte("[FETCH-VERSION][FAIL:HTTP-STATUS] " + targetURL + " " + strconv.Itoa(body.StatusCode))
		return "fail"
	}

	// parse json
	var fw firmwareStatus
	if err = json.Unmarshal(data, &fw); err != nil {
		displayChan <- []byte("[PARSE-VERSION][FAIL:JSON-PARSER] " + targetURL + err.Error())
		return "fail"
	}
	return fw.Product.ProductVersion
}

// installPKG
func installPKG(config *OPNCall, server, pkg string) error {

	// guard against empty package names (e.g. from malformed plugin lists)
	if pkg == "" {
		return errors.New("[INSTALL-PKG][EMPTY-PACKAGE-NAME]")
	}

	// parse & assemble target url
	targetURL := "https://" + server + _apiInstallPKG + pkg
	if _, err := url.Parse(targetURL); err != nil {
		return errors.New("[INSTALL-PKG][UNABLE-TO-PARSE-TARGET-URL]" + targetURL + " " + err.Error())
	}

	// setup request: empty JSON body, the package is encoded in the URL path
	req, err := http.NewRequest("POST", targetURL, strings.NewReader("{}"))
	if err != nil {
		return errors.New("[INSTALL-PKG][UNABLE-TO-CREATE-HTTP-REQUEST]" + targetURL + " " + err.Error())
	}
	req.Header.Set("User-Agent", _userAgent)
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(config.Key, config.Secret)

	// connect
	client := opnClient(config, 4)
	body, err := client.Do(req)
	if err != nil {
		displayChan <- []byte("[INSTALL-PKG][FAIL:TLS-CONNECT] " + targetURL + " " + err.Error())
		return errors.New("[UNABLE-TO-TLS-CONNECT-SERVER]")
	}

	// read body
	defer func() { _ = body.Body.Close() }()
	msg, err := io.ReadAll(body.Body)
	if err != nil {
		displayChan <- []byte("[INSTALL-PKG][FAIL:READ-BODY] " + targetURL + " " + err.Error())
		return errors.New("[ERROR-WHILE-READ-ANSWER-BODY] " + string(msg))
	}
	if body.StatusCode < 200 || body.StatusCode > 299 {
		displayChan <- []byte("[INSTALL-PKG][FAIL:HTTP-STATUS] " + targetURL + " " + strconv.Itoa(body.StatusCode) + " " + string(msg))
		return errors.New("[INSTALL-PKG][HTTP-STATUS] " + strconv.Itoa(body.StatusCode))
	}
	if config.Debug {
		displayChan <- []byte("[INSTALL-PKG][OK][FINISH] " + targetURL + " -> " + string(msg))
	}
	time.Sleep(12 * time.Second) // wait for action to finish
	return nil
}

// fetchOPN retrieves the xml, validates it, and unmarshals it into an Opnsense
// object. The raw XML payload is returned as well so the caller (actionOPN)
// can reuse it for the backup check-in instead of downloading the same
// document a second time.
func fetchOPN(server string, config *OPNCall) (opn *Opnsense, masterXML []byte, err error) {

	// fetch current XML config from server
	masterXML, err = fetchXML(server, config)
	if err != nil {
		displayChan <- []byte("[ERROR][FAIL:UNABLE-TO-FETCH] " + server)
		return opn, nil, err
	}

	// validate XML
	if !isValidXML(string(masterXML)) {
		return opn, nil, errors.New("[INVALID-XML-FILE]")
	}

	// xml unmarshal
	if err = xml.Unmarshal(masterXML, &opn); err != nil {
		displayChan <- []byte("[ERROR][XML-PARSE]" + server)
		return opn, nil, err
	}

	// verify opnborg schema completeness, re-encode xml
	_, err = xml.MarshalIndent(&opn, " ", "  ")
	if err != nil {
		displayChan <- []byte("[ERROR][XML-RE-ENCODE]" + server)
		return opn, nil, err
	}

	return opn, masterXML, nil
}

// fetchXML file from target server. The download is retried on a fresh
// TCP+TLS connection (up to _fetchMaxAttempts) whenever the first stream
// comes back transiently broken: a truncated or surplus-framed body, a
// mid-stream reset, or an HTTP 5xx are all per-connection conditions that a
// new stream fixes; definitive 4xx API rejections are returned immediately.
// Each attempt reports its own diagnosis in the display log (status code,
// declared Content-Length vs bytes received), so a failing appliance is
// self-describing at the default log level - no OPN_DEBUG needed.
func fetchXML(server string, config *OPNCall) (data []byte, err error) {

	// parse & assemble target url
	targetURL := "https://" + server + _apiBackupXML
	if _, err = url.Parse(targetURL); err != nil {
		displayChan <- []byte("[FETCH][FAIL:UNABLE-TO-PARSE-TARGET-URL] " + targetURL)
		return nil, errors.New("[UNABLE-TO-PARSE-TARGET-URL] " + err.Error())
	}

	// fetch with retry: a fresh transport (and therefore a fresh TCP+TLS
	// stream) is built per attempt, so a broken connection can never leak
	// its state into the next try.
	var lastErr error
	for attempt := 1; attempt <= _fetchMaxAttempts; attempt++ {
		data, err = fetchXMLAttempt(targetURL, server, config, attempt)
		if err == nil {
			return data, nil
		}
		lastErr = err
		if attempt == _fetchMaxAttempts || !fetchRetryable(err) {
			break
		}
		displayChan <- []byte("[FETCH][WARN][RETRY:TRANSIENT-STREAM] " + server +
			" [ATTEMPT] " + strconv.Itoa(attempt) + "/" + strconv.Itoa(_fetchMaxAttempts) +
			" [REASON] " + err.Error())
		time.Sleep(_fetchRetrySleep * time.Duration(attempt))
	}
	return nil, lastErr
}

// fetchXMLAttempt performs a single backup XML download attempt.
func fetchXMLAttempt(targetURL, server string, config *OPNCall, attempt int) ([]byte, error) {

	// setup request
	req, err := getRequest(targetURL, _userAgent)
	if err != nil {
		displayChan <- []byte("[FETCH][FAIL:SETUP-URL] " + targetURL)
		return nil, &fetchError{err: errors.New("[UNABLE-TO-SETUP-TARGET-URL] " + err.Error()), retryable: false}
	}
	req.SetBasicAuth(config.Key, config.Secret)

	// connect: fresh transport per attempt (see fetchXML), TLS version range
	// from config (OPN_TLS_MIN_VERSION / OPN_TLS_MAX_VERSION), the request
	// carries Connection: close so the transport never pools this stream and
	// surplus record-stream bytes can never poison a follow-up request.
	client := opnClient(config, _fetchTimeoutSec)
	body, err := client.Do(req)
	if err != nil {
		displayChan <- []byte("[FETCH][FAIL:TLS-CONNECT] " + targetURL + " [ATTEMPT] " +
			strconv.Itoa(attempt) + "/" + strconv.Itoa(_fetchMaxAttempts) + " " + err.Error())
		return nil, &fetchError{err: errors.New("[UNABLE-TO-TLS-CONNECT-SERVER] " + err.Error()), retryable: true}
	}

	// read, validate & return full xml body
	defer func() { _ = body.Body.Close() }()
	data, err := io.ReadAll(body.Body)
	if err != nil {
		displayChan <- []byte("[FETCH][FAIL:READ-BODY] " + targetURL + " [RECEIVED] " +
			strconv.Itoa(len(data)) + "B [DECLARED] " + strconv.FormatInt(body.ContentLength, 10) + "B " + err.Error())
		return nil, &fetchError{err: errors.New("[UNABLE-TO-READ-XML-BODY] " + err.Error()), retryable: true}
	}
	if config.Debug {
		displayChan <- []byte("[FETCH][DEBUG] " + targetURL + " [ATTEMPT] " +
			strconv.Itoa(attempt) + "/" + strconv.Itoa(_fetchMaxAttempts) +
			" [STATUS] " + strconv.Itoa(body.StatusCode) +
			" [DECLARED] " + strconv.FormatInt(body.ContentLength, 10) + "B [RECEIVED] " + strconv.Itoa(len(data)) + "B")
	}

	// a non-2xx answer is an auth, permission or api failure: an auth page
	// must not be mislabelled as a truncated body. 5xx is server-side and
	// transient, everything below is definitive and not retried.
	if body.StatusCode < 200 || body.StatusCode > 299 {
		displayChan <- []byte("[FETCH][FAIL:HTTP-STATUS] " + targetURL +
			" [STATUS] " + strconv.Itoa(body.StatusCode) +
			" [DECLARED] " + strconv.FormatInt(body.ContentLength, 10) + "B [RECEIVED] " + strconv.Itoa(len(data)) + "B")
		return nil, &fetchError{err: errors.New("[HTTP-STATUS] " + strconv.Itoa(body.StatusCode) +
			" [UNABLE-TO-FETCH-XML]"), retryable: body.StatusCode >= 500}
	}

	// truncation guard: the appliance framing must match the delivered
	// payload. A short body (declared more than delivered) or a surplus-framed
	// stream (the transport stops at Content-Length while the appliance keeps
	// streaming the tail, the FreeBSD kTLS/nginx record-split mode) both end
	// up here as a truncated document and are retried on a fresh stream.
	if body.ContentLength >= 0 && int64(len(data)) != body.ContentLength {
		displayChan <- []byte("[FETCH][FAIL:TRUNCATED-XML-BODY] " + targetURL +
			" [DECLARED] " + strconv.FormatInt(body.ContentLength, 10) + "B [RECEIVED] " + strconv.Itoa(len(data)) + "B")
		return nil, &fetchError{err: errors.New("[TRUNCATED-XML-BODY] [DECLARED] " +
			strconv.FormatInt(body.ContentLength, 10) + "B [RECEIVED] " + strconv.Itoa(len(data)) + "B"),
			retryable: true}
	}

	// validate: a body that validates is the only accepted result
	if !isValidXML(string(data)) {
		displayChan <- []byte("[FETCH][ERROR][FAIL:XML-VALIDATION] " + targetURL +
			" [DECLARED] " + strconv.FormatInt(body.ContentLength, 10) + "B [RECEIVED] " + strconv.Itoa(len(data)) + "B")
		return nil, &fetchError{err: errors.New("[INVALID-XML-FILE] [TRUNCATED-OR-CORRUPT-BODY] [RECEIVED] " +
			strconv.Itoa(len(data)) + "B"), retryable: true}
	}
	return data, nil
}

// parseTLSVersion parses an OPN_TLS_{MIN,MAX}_VERSION value into a
// crypto/tls version constant. Accepted spellings: "1.2", "1.3",
// "tls12", "tls13", "tls1.2", "tls1.3" (case-insensitive, surrounding
// whitespace tolerated). Anything else is a setup-fatal error.
func parseTLSVersion(env, val string) (uint16, error) {
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "1.2", "tls12", "tls1.2":
		return tls.VersionTLS12, nil
	case "1.3", "tls13", "tls1.3":
		return tls.VersionTLS13, nil
	}
	return 0, errors.New("[SETUP][" + env + "][UNSUPPORTED-TLS-VERSION] '" + val + "' (supported: 1.2, 1.3)")
}

// tlsVersionString renders a crypto/tls version constant for log output.
func tlsVersionString(ver uint16) string {
	switch ver {
	case tls.VersionTLS12:
		return "1.2"
	case tls.VersionTLS13:
		return "1.3"
	}
	return strconv.Itoa(int(ver))
}

// getTlsConf harden tls object settings. The negotiated version range is
// tunable via OPN_TLS_MIN_VERSION / OPN_TLS_MAX_VERSION (default: TLS 1.3
// only, the OPNsense API standard). CipherSuites is deliberately left
// unset for TLS 1.3 (Go ignores it there and always enables the standard
// AES-GCM + ChaCha20-Poly1305 suites, the FreeBSD kTLS record-offload set);
// when the operator lowers the range into TLS 1.2 - the escape hatch for an
// appliance whose kTLS TLS 1.3 record stream corrupts the backup download -
// a ChaCha20-Poly1305 TLS 1.2 suite pair is pinned (the previous code pinned
// the TLS-1.3-only suite id TLS_CHACHA20_POLY1305_SHA256, which would break
// every TLS 1.2 handshake with a "no common cipher suite" failure).
func getTlsConf(config *OPNCall) *tls.Config {
	tlsConfig := &tls.Config{
		InsecureSkipVerify:     true,
		SessionTicketsDisabled: true,
		Renegotiation:          0,
		MinVersion:             tls.VersionTLS13,
		MaxVersion:             tls.VersionTLS13,
		CurvePreferences:       []tls.CurveID{tls.X25519},
	}
	if config.TLSMin != 0 {
		tlsConfig.MinVersion = config.TLSMin
	}
	if config.TLSMax != 0 {
		tlsConfig.MaxVersion = config.TLSMax
	}
	if tlsConfig.MaxVersion <= tls.VersionTLS12 {
		tlsConfig.CipherSuites = []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
		}
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

// opnClient assembles a hardened HTTP client for OPNsense API calls: TLS 1.3
// config (with optional keypin), compression disabled, and a caller-supplied
// per-call timeout. It collapses the getTlsConf → getTransport → getClient →
// set-Timeout sequence previously duplicated across fetchXML, fetchVersion
// and installPKG.
func opnClient(config *OPNCall, timeoutSec int) *http.Client {
	return &http.Client{
		CheckRedirect: nil,
		Jar:           nil,
		Transport:     getTransport(getTlsConf(config)),
		Timeout:       time.Duration(timeoutSec) * time.Second,
	}
}

// getRequest setup hardened http request. Every API fetch carries
// Connection: close: the transport never pools these streams, so surplus or
// mis-framed record-stream bytes from a slow appliance close-out can never
// be read as the start of the next request on a reused idle connection.
func getRequest(targetURL, userAgent string) (*http.Request, error) {
	u, err := url.Parse(targetURL)
	if err != nil {
		return nil, err
	}
	return &http.Request{
		Method:     http.MethodGet,
		URL:        u,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header: http.Header{
			"User-Agent": []string{userAgent},
		},
		Close: true, // Connection: close - fresh stream per API fetch
	}, nil
}
