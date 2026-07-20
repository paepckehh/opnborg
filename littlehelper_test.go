package opnborg

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// withEnv sets an env var for the duration of the test and restores the
// previous state on cleanup. Tests must not run in parallel with each other
// when they touch the same env var, since os.Environ is process-global.
func withEnv(tb testing.TB, key, value string, set bool) {
	tb.Helper()
	old, had := os.LookupEnv(key)
	if set {
		if err := os.Setenv(key, value); err != nil {
			tb.Fatalf("setenv %s: %v", key, err)
		}
	} else {
		if err := os.Unsetenv(key); err != nil {
			tb.Fatalf("unsetenv %s: %v", key, err)
		}
	}
	tb.Cleanup(func() {
		if had {
			_ = os.Setenv(key, old)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

// ensureDisplayDrained replaces the package-global displayChan with a
// buffered one and drains it in the background so the functions under test
// that send to displayChan do not block. The package init creates
// displayChan with capacity 20; we just spin a reader.
func ensureDisplayDrained(tb testing.TB) {
	tb.Helper()
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-displayChan:
			case <-done:
				return
			}
		}
	}()
	tb.Cleanup(func() { close(done) })
}

// --- littlehelper.go ------------------------------------------------------

func TestPadMonth(t *testing.T) {
	cases := map[string]string{
		"1":   "01",
		"9":   "09",
		"01":  "01",
		"12":  "12",
		"":    "",
		"100": "100",
	}
	for in, want := range cases {
		if got := padMonth(in); got != want {
			t.Errorf("padMonth(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsEnvPresenceBased(t *testing.T) {
	// Unset => false.
	withEnv(t, "OPNBORG_TEST_ABSENT", "", false)
	if isEnv("OPNBORG_TEST_ABSENT") {
		t.Fatalf("isEnv true for unset var")
	}
	// Any non-empty value => true, even the literal "false".
	for _, v := range []string{"1", "0", "false", "no", "whatever"} {
		withEnv(t, "OPNBORG_TEST_PRESENCE", v, true)
		if !isEnv("OPNBORG_TEST_PRESENCE") {
			t.Errorf("isEnv = false for value %q (presence-based semantics)", v)
		}
	}
	// Empty string value => treated as not-set per isEnv implementation.
	withEnv(t, "OPNBORG_TEST_EMPTY", "", true)
	if isEnv("OPNBORG_TEST_EMPTY") {
		t.Errorf("isEnv = true for empty value; implementation treats empty as false")
	}
}

func TestIsValidXML(t *testing.T) {
	cases := map[string]bool{
		"<opnsense></opnsense>": true,
		"<opnsense><system><hostname>fw</hostname></system></opnsense>": true,
		"<?xml version=\"1.0\"?><a><b/></a>":                            true,
		"not xml at all":                                                false,
		"<open>missing close":                                           false,
		"":                                                              false,
		"<a>&invalidentity</a>":                                         false,
	}
	for in, want := range cases {
		if got := isValidXML(in); got != want {
			t.Errorf("isValidXML(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestCheckURL(t *testing.T) {
	// Unset var => nil, nil.
	withEnv(t, "OPNBORG_TEST_URL_ABSENT", "", false)
	if got, err := checkURL("OPNBORG_TEST_URL_ABSENT"); err != nil || got != nil {
		t.Fatalf("absent var: got %v, err %v", got, err)
	}
	// Set valid URL => parsed.
	withEnv(t, "OPNBORG_TEST_URL_OK", "https://10.0.0.1:8443/", true)
	got, err := checkURL("OPNBORG_TEST_URL_OK")
	if err != nil || got == nil || got.Scheme != "https" || got.Host != "10.0.0.1:8443" {
		t.Fatalf("valid url: got %+v err %v", got, err)
	}
	// Invalid URL => error, nil.
	withEnv(t, "OPNBORG_TEST_URL_BAD", "://missing-scheme", true)
	if _, err := checkURL("OPNBORG_TEST_URL_BAD"); err == nil {
		t.Fatalf("expected error for invalid url")
	}
}

func TestCheckPreURL(t *testing.T) {
	base, err := url.Parse("https://grafana.example.com")
	if err != nil {
		t.Fatal(err)
	}
	withEnv(t, "OPNBORG_DASH", "abc-123", true)
	got, err := checkPreURL(base, "/d/", "OPNBORG_DASH")
	if err != nil || got == nil {
		t.Fatalf("got %v err %v", got, err)
	}
	if want := "https://grafana.example.com/d/abc-123"; got.String() != want {
		t.Errorf("got %q want %q", got.String(), want)
	}
	// Empty env var still produces the prefix-only URL (no error).
	withEnv(t, "OPNBORG_DASH", "", true)
	got, err = checkPreURL(base, "/d/", "OPNBORG_DASH")
	if err != nil {
		t.Fatal(err)
	}
	if want := "https://grafana.example.com/d/"; got.String() != want {
		t.Errorf("empty env: got %q want %q", got.String(), want)
	}
}

// --- transport.go: keypin helpers -----------------------------------------

// selfSignedCert returns a DER-encoded self-signed cert + its leaf x509.Certificate
// so we can exercise keyPinBase64 / pinVerifyState without network IO.
func selfSignedCert(t *testing.T) (*x509.Certificate, []byte) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-opn"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	return cert, der
}

func TestKeyPinBase64(t *testing.T) {
	cert, _ := selfSignedCert(t)
	got := keyPinBase64(cert)
	wantRaw := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	want := base64.StdEncoding.EncodeToString(wantRaw[:])
	if got != want {
		t.Errorf("keyPinBase64 mismatch: got %q want %q", got, want)
	}
	// Must be valid base64 of exactly 32 bytes.
	dec, err := base64.StdEncoding.DecodeString(got)
	if err != nil {
		t.Fatalf("decoded pin is not valid base64: %v", err)
	}
	if len(dec) != 32 {
		t.Errorf("decoded pin length = %d, want 32", len(dec))
	}
}

func TestPinVerifyState(t *testing.T) {
	cert, _ := selfSignedCert(t)
	pin := keyPinBase64(cert)
	state := &tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}
	if !pinVerifyState(pin, state) {
		t.Errorf("pinVerifyState = false for matching pin")
	}
	if pinVerifyState("not-the-right-pin=", state) {
		t.Errorf("pinVerifyState = true for mismatched pin")
	}
	// No certs in state => false.
	emptyState := &tls.ConnectionState{}
	if pinVerifyState(pin, emptyState) {
		t.Errorf("pinVerifyState = true with no peer certs")
	}
}

func TestGetTlsConf(t *testing.T) {
	t.Run("no keypin", func(t *testing.T) {
		c := getTlsConf(&OPNCall{})
		if c == nil {
			t.Fatal("nil config")
		}
		if !c.InsecureSkipVerify {
			t.Error("expected InsecureSkipVerify=true by default")
		}
		if c.MinVersion != tls.VersionTLS13 || c.MaxVersion != tls.VersionTLS13 {
			t.Error("expected TLS 1.3 only")
		}
		if c.VerifyConnection != nil {
			t.Error("VerifyConnection should be nil without keypin")
		}
	})
	t.Run("with keypin", func(t *testing.T) {
		c := getTlsConf(&OPNCall{TLSKeyPin: "FezOCC3qZFzBmD5xRKtDoLgK445Kr0DeJBj2TWVvR9M="})
		if c.VerifyConnection == nil {
			t.Error("VerifyConnection should be set when TLSKeyPin configured")
		}
	})
}

// --- rsyslog-clientconf.go ----------------------------------------------

func TestGetLogConf(t *testing.T) {
	opn := getLogConf([]string{"192.168.0.10", "5140"})
	d := opn.OPNsense.Syslog.Destinations.Destination
	if d.Enabled != "1" {
		t.Errorf("Enabled = %q want 1", d.Enabled)
	}
	if d.Transport != "udp4" {
		t.Errorf("Transport = %q want udp4", d.Transport)
	}
	if d.Hostname != "192.168.0.10" {
		t.Errorf("Hostname = %q want 192.168.0.10", d.Hostname)
	}
	if d.Port != "5140" {
		t.Errorf("Port = %q want 5140", d.Port)
	}
	if d.Rfc5424 != "1" {
		t.Errorf("Rfc5424 = %q want 1", d.Rfc5424)
	}
	if d.Uuid != "ce2c4ccb-77da-4e3f-96bd-7c3fca832bc7" {
		t.Errorf("Uuid = %q want fixed uuid", d.Uuid)
	}
}

func TestCompareLogConf(t *testing.T) {
	const server = "opn01.lan"
	srv := []string{"192.168.0.10", "5140"}

	// A correctly-configured destination should produce no error.
	good := getLogConf(srv)
	if err := compareLogConf(server, srv, good); err != nil {
		t.Errorf("matched config returned error: %v", err)
	}

	// Each field violation must surface a distinct error.
	badEnabled := getLogConf(srv)
	badEnabled.OPNsense.Syslog.Destinations.Destination.Enabled = "0"
	if err := compareLogConf(server, srv, badEnabled); err == nil {
		t.Error("expected error for Enabled mismatch")
	}

	badTransport := getLogConf(srv)
	badTransport.OPNsense.Syslog.Destinations.Destination.Transport = "tcp"
	if err := compareLogConf(server, srv, badTransport); err == nil {
		t.Error("expected error for Transport mismatch")
	}

	badHost := getLogConf(srv)
	badHost.OPNsense.Syslog.Destinations.Destination.Hostname = "10.0.0.1"
	if err := compareLogConf(server, srv, badHost); err == nil {
		t.Error("expected error for Hostname mismatch")
	}

	badPort := getLogConf(srv)
	badPort.OPNsense.Syslog.Destinations.Destination.Port = "9999"
	if err := compareLogConf(server, srv, badPort); err == nil {
		t.Error("expected error for Port mismatch")
	}

	badRfc := getLogConf(srv)
	badRfc.OPNsense.Syslog.Destinations.Destination.Rfc5424 = "0"
	if err := compareLogConf(server, srv, badRfc); err == nil {
		t.Error("expected error for Rfc5424 mismatch")
	}
}

// --- httpd-handler.go: getPKG / getNavi ---------------------------------

func TestGetPKGEmpty(t *testing.T) {
	// syncPKG is short / empty by default at package scope.
	saved := syncPKG
	t.Cleanup(func() { syncPKG = saved })
	syncPKG = ""
	if got := getPKG(); got != "" {
		t.Errorf("getPKG() with empty syncPKG = %q, want empty", got)
	}
	syncPKG = "os-"
	if got := getPKG(); got != "" {
		t.Errorf("getPKG() with len<5 syncPKG = %q, want empty", got)
	}
}

func TestGetPKGRenders(t *testing.T) {
	saved := syncPKG
	t.Cleanup(func() { syncPKG = saved })
	syncPKG = "os-foo,os-bar,baz"
	got := getPKG()
	if !strings.Contains(got, "BorgSYNC") {
		t.Errorf("missing BorgSYNC heading: %q", got)
	}
	if !strings.Contains(got, "foo") || !strings.Contains(got, "bar") {
		t.Errorf("missing package names: %q", got)
	}
	if strings.Contains(got, "os-foo") {
		t.Errorf("os- prefix should be stripped: %q", got)
	}
	if !strings.Contains(got, "foo / bar") {
		t.Errorf("packages should be slash-separated: %q", got)
	}
}

func TestGetNaviEmpty(t *testing.T) {
	// No WebUI globals set => empty.
	saved := struct {
		prom, grafana, fbsd, haproxy, unifiG, wazuh *url.URL
	}{prometheusWebUI, grafanaWebUI, grafanaFreeBSD, grafanaHAProxy, grafanaUnifi, wazuhWebUI}
	t.Cleanup(func() {
		prometheusWebUI, grafanaWebUI, grafanaFreeBSD, grafanaHAProxy, grafanaUnifi, wazuhWebUI =
			saved.prom, saved.grafana, saved.fbsd, saved.haproxy, saved.unifiG, saved.wazuh
	})
	prometheusWebUI, grafanaWebUI, grafanaFreeBSD, grafanaHAProxy, grafanaUnifi, wazuhWebUI = nil, nil, nil, nil, nil, nil
	if got := getNavi(); got != "" {
		t.Errorf("getNavi() with no WebUIs = %q, want empty", got)
	}
}

func TestGetNaviPrometheusOnly(t *testing.T) {
	saved := prometheusWebUI
	t.Cleanup(func() { prometheusWebUI = saved })
	u, err := url.Parse("https://prom.example.com")
	if err != nil {
		t.Fatal(err)
	}
	prometheusWebUI = u
	got := getNavi()
	if !strings.Contains(got, "https://prom.example.com/targets?search=") {
		t.Errorf("missing prometheus link: %q", got)
	}
	if !strings.Contains(got, "[ PrometheusDB ]") {
		t.Errorf("missing prometheus label: %q", got)
	}
}

// --- store.go: checkIntoStore + lastSum ----------------------------------

func TestCheckIntoStore(t *testing.T) {
	ensureDisplayDrained(t)

	// checkIntoStore chdir's into the store root, so we must run each
	// subtest in its own goroutine-resumable cwd. Save and restore the
	// process cwd explicitly.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	tmp := t.TempDir()
	server := "opn01.lan"
	config := &OPNCall{Path: tmp}

	payload := []byte("<opnsense><system><hostname>opn01</hostname></system></opnsense>")
	ts := time.Date(2024, 5, 17, 12, 34, 56, 0, time.UTC)
	sum := sha256.Sum256(payload)

	if err := checkIntoStore(config, server, "xml", payload, ts, sum); err != nil {
		t.Fatalf("checkIntoStore: %v", err)
	}

	// current.xml should exist in the per-server dir and match the payload.
	currentFile := filepath.Join(tmp, server, "current.xml")
	got, err := os.ReadFile(currentFile)
	if err != nil {
		t.Fatalf("read current.xml: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("current.xml payload mismatch")
	}

	// CONFIG-CURRENT symlink target must resolve to the archive file.
	linkPath := filepath.Join(tmp, server, _current)
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("readlink %s: %v", linkPath, err)
	}
	if !strings.HasSuffix(target, ".xml") {
		t.Errorf("CONFIG-CURRENT symlink target %q does not look like an xml archive", target)
	}

	// sha256.db should contain exactly one line referencing the archive name.
	hashData, err := os.ReadFile(filepath.Join(tmp, server, _hashFile))
	if err != nil {
		t.Fatalf("read hashfile: %v", err)
	}
	archiveName := ts.UTC().Format("20060102T150405Z") + "-" + server + ".xml"
	if !strings.Contains(string(hashData), archiveName) {
		t.Errorf("hashfile does not reference archive %q: %s", archiveName, hashData)
	}
	// The stored base64 sum must match the input sum.
	wantB64 := base64.StdEncoding.EncodeToString(sum[:])
	if !strings.Contains(string(hashData), wantB64) {
		t.Errorf("hashfile does not contain expected base64 sum: %s", hashData)
	}

	// lastSum must equal sha256(payload) now (it reads CONFIG-CURRENT).
	gotSum := lastSum(config, server)
	if gotSum != sum {
		t.Errorf("lastSum mismatch: got %x want %x", gotSum, sum)
	}
}

func TestCheckIntoStoreRotation(t *testing.T) {
	ensureDisplayDrained(t)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	tmp := t.TempDir()
	server := "opn02.lan"
	config := &OPNCall{Path: tmp}

	first := []byte("<opnsense><v>1</v></opnsense>")
	second := []byte("<opnsense><v>2</v></opnsense>")
	ts := time.Date(2024, 5, 17, 12, 34, 56, 0, time.UTC)
	if err := checkIntoStore(config, server, "xml", first, ts, sha256.Sum256(first)); err != nil {
		t.Fatalf("first checkin: %v", err)
	}
	ts2 := ts.Add(time.Minute)
	if err := checkIntoStore(config, server, "xml", second, ts2, sha256.Sum256(second)); err != nil {
		t.Fatalf("second checkin: %v", err)
	}

	// After the second checkin, CONFIG-LAST should hold the previous payload
	// and CONFIG-CURRENT the new one.
	lastData, err := os.ReadFile(filepath.Join(tmp, server, _last))
	if err != nil {
		t.Fatalf("read CONFIG-LAST: %v", err)
	}
	if string(lastData) != string(first) {
		t.Errorf("CONFIG-LAST does not contain first payload")
	}
	curData, err := os.ReadFile(filepath.Join(tmp, server, _current))
	if err != nil {
		t.Fatalf("read CONFIG-CURRENT: %v", err)
	}
	if string(curData) != string(second) {
		t.Errorf("CONFIG-CURRENT does not contain second payload")
	}

	// sha256.db should now have two lines.
	hashData, err := os.ReadFile(filepath.Join(tmp, server, _hashFile))
	if err != nil {
		t.Fatalf("read hashfile: %v", err)
	}
	if got := strings.Count(string(hashData), "\n"); got != 2 {
		t.Errorf("hashfile line count = %d, want 2", got)
	}
}

// TestConcurrentHiveStatusMutations exercises the hiveMutex guard around the
// package-global `hive` slice. It is a race detector bait rather than a
// functional correctness check; run with `-race`.
func TestConcurrentHiveStatusMutations(t *testing.T) {
	ensureDisplayDrained(t)

	savedHive := hive
	t.Cleanup(func() { hive = savedHive })
	hive = make([]string, 5)

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			hiveMutex.Lock()
			defer hiveMutex.Unlock()
			hive[0] = "<td>x</td>"
		}()
		go func(id int) {
			defer wg.Done()
			_ = getHive() // acquires hiveMutex
		}(i)
	}
	wg.Wait()
}

// --- compress.go ---------------------------------------------------------

func TestCompressLevelGZIPRoundtrip(t *testing.T) {
	ensureDisplayDrained(t)
	in := bytes.Repeat([]byte("opnborg"), 500) // > MTU
	out := compressLevel(in, gzip.NewWriterLevel)
	if len(out) == 0 {
		t.Fatal("gzip output empty")
	}
	if bytes.Equal(out, in) {
		t.Fatal("gzip did not compress")
	}
	dec, err := gzip.NewReader(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	rt, err := io.ReadAll(dec)
	if err != nil {
		t.Fatalf("gzip readall: %v", err)
	}
	if !bytes.Equal(rt, in) {
		t.Error("gzip roundtrip mismatch")
	}
}

func TestCompressLevelDeflateRoundtrip(t *testing.T) {
	ensureDisplayDrained(t)
	in := bytes.Repeat([]byte("opnborg"), 500)
	out := compressLevel(in, zlib.NewWriterLevel)
	if len(out) == 0 {
		t.Fatal("deflate output empty")
	}
	if bytes.Equal(out, in) {
		t.Fatal("deflate did not compress")
	}
	dec, _ := zlib.NewReader(bytes.NewReader(out))
	rt, err := io.ReadAll(dec)
	if err != nil {
		t.Fatalf("deflate readall: %v", err)
	}
	if !bytes.Equal(rt, in) {
		t.Error("deflate roundtrip mismatch")
	}
}

func TestCompressLevelInvalidLevelReturnsData(t *testing.T) {
	ensureDisplayDrained(t)
	// An invalid level (out of gzip's supported range) causes the constructor
	// to fail; compressLevel must then return the input unchanged.
	in := []byte("payload")
	got := compressLevel(in, func(w io.Writer, _ int) (*gzip.Writer, error) {
		return nil, errors.New("boom")
	})
	if !bytes.Equal(got, in) {
		t.Errorf("expected fallback to original data, got %q", got)
	}
}

func TestWriteTransportCompressedPageSmallPayload(t *testing.T) {
	ensureDisplayDrained(t)
	q := &http.Request{Header: http.Header{}}
	rr := httptestOK(t)
	writeTransportCompressedPage("short", rr, q, true)
	if got := rr.Body.String(); got != "short" {
		t.Errorf("body = %q want %q", got, "short")
	}
	if enc := rr.Header().Get("Content-Encoding"); enc != "" {
		t.Errorf("small payload should not be compressed, got %q", enc)
	}
}

func TestWriteTransportCompressedPageGzip(t *testing.T) {
	ensureDisplayDrained(t)
	q := &http.Request{Header: http.Header{"Accept-Encoding": []string{"gzip"}}}
	rr := httptestOK(t)
	page := strings.Repeat("x", _compressMTU+1)
	writeTransportCompressedPage(page, rr, q, true)
	if rr.Header().Get("Content-Encoding") != "gzip" {
		t.Error("expected gzip Content-Encoding")
	}
	dec, err := gzip.NewReader(bytes.NewReader(rr.Body.Bytes()))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	rt, _ := io.ReadAll(dec)
	if string(rt) != page {
		t.Error("gzip roundtrip mismatch")
	}
}

func TestWriteTransportCompressedPageDeflate(t *testing.T) {
	ensureDisplayDrained(t)
	q := &http.Request{Header: http.Header{"Accept-Encoding": []string{"deflate"}}}
	rr := httptestOK(t)
	page := strings.Repeat("y", _compressMTU+1)
	writeTransportCompressedPage(page, rr, q, true)
	if rr.Header().Get("Content-Encoding") != "deflate" {
		t.Error("expected deflate Content-Encoding")
	}
	dec, _ := zlib.NewReader(bytes.NewReader(rr.Body.Bytes()))
	rt, _ := io.ReadAll(dec)
	if string(rt) != page {
		t.Error("deflate roundtrip mismatch")
	}
}

func TestWriteTransportCompressedPageNoCompressionWhenDisabled(t *testing.T) {
	ensureDisplayDrained(t)
	q := &http.Request{Header: http.Header{"Accept-Encoding": []string{"gzip"}}}
	rr := httptestOK(t)
	page := strings.Repeat("z", _compressMTU+1)
	writeTransportCompressedPage(page, rr, q, false)
	if rr.Header().Get("Content-Encoding") != "" {
		t.Error("compression should be skipped when tryCompress is false")
	}
	if rr.Body.String() != page {
		t.Error("body mismatch")
	}
}

// httptestOK returns a configured httptest.ResponseRecorder.
func httptestOK(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	return httptest.NewRecorder()
}

// --- rsyslog-clientconf.go: mismatchErr --------------------------------

func TestMismatchErrFormat(t *testing.T) {
	got := mismatchErr("[LABEL]", "opn01.lan", "0", "1")
	want := "[LABEL] opn01.lan -> have: 0 need: 1"
	if got.Error() != want {
		t.Errorf("got %q want %q", got.Error(), want)
	}
}

func TestGetLogConfUsesConstants(t *testing.T) {
	opn := getLogConf([]string{"10.0.0.1", "514"})
	d := opn.OPNsense.Syslog.Destinations.Destination
	if d.Uuid != _syslogUUID {
		t.Errorf("uuid = %q want %q", d.Uuid, _syslogUUID)
	}
	if d.Level != _syslogLevel {
		t.Errorf("level mismatch")
	}
	if d.Facility != _syslogFacility {
		t.Errorf("facility mismatch")
	}
	if d.Program != _syslogProgram {
		t.Errorf("program mismatch")
	}
	if d.Description != _syslogDesc {
		t.Errorf("description mismatch")
	}
	if d.Enabled != _syslogEnabled || d.Transport != _syslogTransport || d.Rfc5424 != _syslogRfc5424 {
		t.Errorf("fixed-value field mismatch")
	}
}

// --- store.go: logBackupErr ----------------------------------------------

func TestLogBackupErrSendsToDisplayChan(t *testing.T) {
	// Replace displayChan with a buffered one we control so we can assert
	// the exact payload without racing the package init reader.
	saved := displayChan
	t.Cleanup(func() { displayChan = saved })
	ch := make(chan []byte, 1)
	displayChan = ch

	logBackupErr("FAIL:TEST", "ctx-123")
	select {
	case got := <-ch:
		want := "[BACKUP][ERROR][FAIL:TEST] ctx-123"
		if string(got) != want {
			t.Errorf("got %q want %q", string(got), want)
		}
	case <-time.After(time.Second):
		t.Fatal("displayChan did not receive the error message")
	}
}
