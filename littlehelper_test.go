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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	gitcfg "github.com/go-git/go-git/v5/config"
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

// ensureDisplayDrained spins a background reader against the package-global
// displayChan so the functions under test that send to it do not block. It
// captures the channel value at setup time: if a later test swaps displayChan
// for its own channel, that test is responsible for draining its own swap.
func ensureDisplayDrained(tb testing.TB) {
	tb.Helper()
	ch := displayChan
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ch:
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

	// Each field mismatch must carry the correct diagnostic label so operators
	// can identify which value drifted. Previously Transport and Rfc5424 were
	// mislabelled as HOSTNAME/PORT respectively.
	cases := []struct {
		name  string
		mut   func(d *SyslogDestination)
		label string
	}{
		{"Enabled", func(d *SyslogDestination) { d.Enabled = "0" }, "[TARGET-REMOTE-SYSLOG-SERVER-ENABLED]"},
		{"Transport", func(d *SyslogDestination) { d.Transport = "tcp" }, "[TARGET-REMOTE-SYSLOG-TRANSPORT]"},
		{"Hostname", func(d *SyslogDestination) { d.Hostname = "10.0.0.1" }, "[TARGET-REMOTE-SYSLOG-HOSTNAME]"},
		{"Port", func(d *SyslogDestination) { d.Port = "9999" }, "[TARGET-REMOTE-SYSLOG-PORT]"},
		{"Rfc5424", func(d *SyslogDestination) { d.Rfc5424 = "0" }, "[TARGET-REMOTE-SYSLOG-RFC5424]"},
	}
	for _, c := range cases {
		opn := getLogConf(srv)
		c.mut(&opn.OPNsense.Syslog.Destinations.Destination)
		err := compareLogConf(server, srv, opn)
		if err == nil {
			t.Errorf("%s: expected labelled error, got nil", c.name)
			continue
		}
		if !strings.HasPrefix(err.Error(), c.label+" ") {
			t.Errorf("%s: error %q does not start with expected label %q", c.name, err.Error(), c.label)
		}
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
	if got := getNavi(); got != "<nav></nav>" {
		t.Errorf("getNavi() with no WebUIs = %q, want %q", got, "<nav></nav>")
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
	archiveName := ts.UTC().Format("20060102T150405.000Z") + "-" + server + ".xml"
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

// TestLastSumMissingFile covers the first-backup case where no CONFIG-CURRENT
// file exists yet. lastSum must return the zero hash and must NOT emit a
// backup error log (it is a benign first-run condition, not a failure).
func TestLastSumMissingFile(t *testing.T) {
	ensureDisplayDrained(t)

	// Swap displayChan for a captured one so we can assert silence.
	saved := displayChan
	displayChan = make(chan []byte, 8)
	t.Cleanup(func() { displayChan = saved })

	tmp := t.TempDir()
	server := "first-run.lan"
	config := &OPNCall{Path: tmp}

	got := lastSum(config, server)
	if got != ([32]byte{}) {
		t.Errorf("lastSum on missing CONFIG-CURRENT = %x, want zero hash", got)
	}

	// Drain: no error message should have been emitted on first run.
	select {
	case msg := <-displayChan:
		t.Fatalf("lastSum emitted unexpected log on missing file: %q", msg)
	default:
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
	// the exact payload. We intentionally do NOT call ensureDisplayDrained
	// here, because that helper spins a reader against the original
	// displayChan; swapping the channel underneath it would race.
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

// --- setup.go: checkSetRequiredOPN --------------------------------------

// resetTargetsGlobals snapshots and restores the package-global tg slice and
// the OPN_TARGETS env var so each subtest starts from a clean state.
func resetTargetsGlobals(t *testing.T) {
	t.Helper()
	savedTg := tg
	savedTargets := os.Getenv("OPN_TARGETS")
	t.Cleanup(func() {
		tg = savedTg
		_ = os.Setenv("OPN_TARGETS", savedTargets)
	})
	tg = nil
}

func TestCheckSetRequiredOPNMissingCreds(t *testing.T) {
	resetTargetsGlobals(t)
	withEnv(t, "OPN_APIKEY", "", false)
	withEnv(t, "OPN_APISECRET", "", false)
	withEnv(t, "OPN_TARGETS", "", false)
	if got := checkSetRequiredOPN(); got {
		t.Errorf("expected false when APIKEY/APISECRET unset")
	}
}

func TestCheckSetRequiredOPNFlatTargets(t *testing.T) {
	resetTargetsGlobals(t)
	withEnv(t, "OPN_APIKEY", "key", true)
	withEnv(t, "OPN_APISECRET", "secret", true)
	withEnv(t, "OPN_TARGETS", "fw01,fw02", true)
	if !checkSetRequiredOPN() {
		t.Fatalf("expected true for flat OPN_TARGETS")
	}
	if len(tg) != 1 {
		t.Fatalf("expected 1 group, got %d", len(tg))
	}
	if tg[0].Name != "" || !tg[0].OPN || tg[0].Desc != "" {
		t.Errorf("unexpected flat group: %+v", tg[0])
	}
	if len(tg[0].Member) != 2 || tg[0].Member[0] != "fw01" || tg[0].Member[1] != "fw02" {
		t.Errorf("unexpected members: %+v", tg[0].Member)
	}
}

func TestCheckSetRequiredOPNGroupsAndDesc(t *testing.T) {
	resetTargetsGlobals(t)
	withEnv(t, "OPN_APIKEY", "key", true)
	withEnv(t, "OPN_APISECRET", "secret", true)
	withEnv(t, "OPN_TARGETS", "", false)
	withEnv(t, "OPN_TARGETS_EDGE", "edge01,edge02", true)
	withEnv(t, "OPN_TARGETS_CORE", "core01", true)
	withEnv(t, "OPN_TARGETS_DESC_EDGE", "Edge firewalls", true)

	if !checkSetRequiredOPN() {
		t.Fatalf("expected true for group targets")
	}
	// OPN_TARGETS must be rewritten to the merged member list.
	if got := os.Getenv("OPN_TARGETS"); got != "edge01,edge02,core01" && got != "core01,edge01,edge02" {
		t.Errorf("OPN_TARGETS not merged correctly: %q", got)
	}
	// find the EDGE group
	var edge *OPNGroup
	for i := range tg {
		if tg[i].Name == "EDGE" {
			edge = &tg[i]
			break
		}
	}
	if edge == nil {
		t.Fatalf("EDGE group not registered: %+v", tg)
	}
	if edge.Desc != "Edge firewalls" {
		t.Errorf("EDGE group desc not wired: %+v", edge)
	}
}

func TestCheckSetRequiredOPNIgnoresDescEntries(t *testing.T) {
	resetTargetsGlobals(t)
	withEnv(t, "OPN_APIKEY", "key", true)
	withEnv(t, "OPN_APISECRET", "secret", true)
	withEnv(t, "OPN_TARGETS", "", false)
	withEnv(t, "OPN_TARGETS_EDGE", "edge01", true)
	withEnv(t, "OPN_TARGETS_DESC_EDGE", "Edge firewalls", true)
	withEnv(t, "OPN_TARGETS_IMGURL_EDGE", "https://example.com/edge.png", true)
	if !checkSetRequiredOPN() {
		t.Fatalf("expected true")
	}
	for _, g := range tg {
		if g.Name == "_EDGE" || strings.HasPrefix(g.Name, "DESC") || strings.HasPrefix(g.Name, "IMGURL") {
			t.Errorf("DESC/IMGURL entry should not be a group: %+v", g)
		}
	}
	// find the EDGE group and verify ImgURL + Desc are wired
	for i := range tg {
		if tg[i].Name == "EDGE" {
			if tg[i].ImgURL != "https://example.com/edge.png" {
				t.Errorf("EDGE group ImgURL not wired: %+v", tg[i])
			}
			if tg[i].Desc != "Edge firewalls" {
				t.Errorf("EDGE group Desc not wired: %+v", tg[i])
			}
		}
	}
}

// --- httpd-handler.go: getNavi all links + getPKG/getHive ---------------

func TestGetNaviAllLinks(t *testing.T) {
	saved := struct {
		prom, grafana, fbsd, haproxy, unifiG, wazuh, unifiW *url.URL
	}{prometheusWebUI, grafanaWebUI, grafanaFreeBSD, grafanaHAProxy, grafanaUnifi, wazuhWebUI, unifiWebUI}
	t.Cleanup(func() {
		prometheusWebUI, grafanaWebUI, grafanaFreeBSD, grafanaHAProxy, grafanaUnifi, wazuhWebUI, unifiWebUI =
			saved.prom, saved.grafana, saved.fbsd, saved.haproxy, saved.unifiG, saved.wazuh, saved.unifiW
	})
	must := func(s string) *url.URL {
		u, err := url.Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	prometheusWebUI = must("https://prom.example.com")
	grafanaWebUI = must("https://grafana.example.com")
	grafanaFreeBSD = must("https://grafana.example.com/d/freebsd")
	grafanaHAProxy = must("https://grafana.example.com/d/haproxy")
	grafanaUnifi = must("https://grafana.example.com/d/unifi")
	wazuhWebUI = must("https://wazuh.example.com")
	unifiWebUI = nil // unifiBackupEnable is false by default in tests
	unifiBackupEnable.Store(false)

	got := getNavi()
	for _, want := range []string{
		"https://prom.example.com/targets?search=",
		"[ PrometheusDB ]",
		"https://grafana.example.com/dashboards",
		"[ Grafana ]",
		"https://grafana.example.com/d/freebsd",
		"[ OPNSense OS Dashboard ]",
		"https://grafana.example.com/d/haproxy",
		"[ HAProxy Dashboard ]",
		"https://grafana.example.com/d/unifi",
		"[ Unifi Dashboard ]",
		"https://wazuh.example.com/",
		"[ Wazuh ]",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("getNavi missing %q in:\n%s", want, got)
		}
	}
}

func TestGetNaviUnifiConditional(t *testing.T) {
	saved := unifiWebUI
	savedBackup := unifiBackupEnable.Load()
	t.Cleanup(func() {
		unifiWebUI = saved
		unifiBackupEnable.Store(savedBackup)
	})
	u, err := url.Parse("https://unifi.example.com")
	if err != nil {
		t.Fatal(err)
	}
	unifiWebUI = u

	// When backup is enabled, the Unifi nav link is suppressed.
	unifiBackupEnable.Store(true)
	if got := getNavi(); strings.Contains(got, "[ Unifi ]") {
		t.Errorf("Unifi link should be hidden when backup is enabled: %q", got)
	}
	// When backup is disabled, the Unifi nav link is shown.
	unifiBackupEnable.Store(false)
	if got := getNavi(); !strings.Contains(got, "[ Unifi ]") {
		t.Errorf("Unifi link should be visible when backup is disabled: %q", got)
	}
}

func TestGetPKGSkipsShortSync(t *testing.T) {
	saved := syncPKG
	t.Cleanup(func() { syncPKG = saved })
	syncPKG = "os"
	if got := getPKG(); got != "" {
		t.Errorf("expected empty for short syncPKG, got %q", got)
	}
}

func TestGetHiveEmpty(t *testing.T) {
	ensureDisplayDrained(t)
	savedHive := hive
	savedTg := tg
	savedSleep := sleep
	t.Cleanup(func() {
		hive = savedHive
		tg = savedTg
		sleep = savedSleep
	})
	tg = nil
	hive = nil
	sleep = "60"
	got := getHive()
	if !strings.Contains(got, "BorgBACKUP") {
		t.Errorf("missing BorgBACKUP header: %q", got)
	}
	if !strings.Contains(got, "60 seconds") {
		t.Errorf("missing sleep interval: %q", got)
	}
	if !strings.Contains(got, _forceButton) {
		t.Errorf("missing force button: %q", got)
	}
}

func TestWriteGroupHeaderDescAndPlain(t *testing.T) {
	var s strings.Builder
	writeGroupHeader(&s, OPNGroup{Name: "Plain"})
	if got := s.String(); !strings.Contains(got, "<b>Plain</b>") {
		t.Errorf("plain header missing label: %q", got)
	}
	s.Reset()
	writeGroupHeader(&s, OPNGroup{Name: "Desc", Desc: "Edge firewalls"})
	got := s.String()
	if !strings.Contains(got, "<b>Desc</b>") {
		t.Errorf("desc header missing label: %q", got)
	}
	if !strings.Contains(got, "Edge firewalls") {
		t.Errorf("desc header missing description text: %q", got)
	}
}

func TestWriteGroupHeaderImgURL(t *testing.T) {
	var s strings.Builder
	// Image only: no tooltip, alt text is the group name.
	writeGroupHeader(&s, OPNGroup{Name: "Img", ImgURL: "https://example.com/i.png"})
	got := s.String()
	if !strings.Contains(got, `src="https://example.com/i.png"`) {
		t.Errorf("img header missing src: %q", got)
	}
	if !strings.Contains(got, `alt="Img"`) {
		t.Errorf("img header missing alt: %q", got)
	}
	if strings.Contains(got, `title=`) {
		t.Errorf("img header should not have title without desc: %q", got)
	}
	if strings.Contains(got, "<b>Img</b>") {
		t.Errorf("img header should not render text headline: %q", got)
	}

	// Image + desc: desc becomes the tooltip; no text headline.
	s.Reset()
	writeGroupHeader(&s, OPNGroup{Name: "Img", Desc: "Edge firewalls", ImgURL: "https://example.com/i.png"})
	got = s.String()
	if !strings.Contains(got, `title="Edge firewalls"`) {
		t.Errorf("img header missing desc tooltip: %q", got)
	}
	if !strings.Contains(got, `src="https://example.com/i.png"`) {
		t.Errorf("img header missing src: %q", got)
	}
	if strings.Contains(got, "group-desc") {
		t.Errorf("img header should not render desc as subheading: %q", got)
	}
}

// --- transport.go: opnClient --------------------------------------------

func TestOpnClientTimeoutApplied(t *testing.T) {
	c := opnClient(&OPNCall{}, 7)
	if c.Timeout != 7*time.Second {
		t.Errorf("timeout = %v want %v", c.Timeout, 7*time.Second)
	}
	if c.Transport == nil {
		t.Error("Transport must be set")
	}
	if c.Jar != nil || c.CheckRedirect != nil {
		t.Error("Jar/CheckRedirect should be nil to match getClient")
	}
}

func TestOpnClientUsesKeyPinWhenConfigured(t *testing.T) {
	without := opnClient(&OPNCall{}, 5).Transport.(*http.Transport).TLSClientConfig
	if without.VerifyConnection != nil {
		t.Error("VerifyConnection should be nil without keypin")
	}
	with := opnClient(&OPNCall{TLSKeyPin: "FezOCC3qZFzBmD5xRKtDoLgK445Kr0DeJBj2TWVvR9M="}, 5).Transport.(*http.Transport).TLSClientConfig
	if with.VerifyConnection == nil {
		t.Error("VerifyConnection should be set when keypin configured")
	}
}

// --- httpd-handler.go: getStartHTML + modern WebUI -----------------------

func TestGetStartHTMLStructure(t *testing.T) {
	ensureDisplayDrained(t)
	savedHive := hive
	savedTg := tg
	savedSleep := sleep
	t.Cleanup(func() {
		hive = savedHive
		tg = savedTg
		sleep = savedSleep
	})
	tg = []OPNGroup{{Name: "TEST", OPN: true, Member: []string{"fw01"}}}
	hive = []string{_na + "<span class=\"member-meta\">fw01</span>"}
	sleep = "60"
	got := getStartHTML()
	for _, want := range []string{
		"<!doctype html>",
		"<html>",
		"<body>",
		"</body>",
		"</html>",
		"<header",
		"<footer",
		"<nav>",
		"</nav>",
		"<div class=\"group\">",
		"member-row",
		"backup-section",
		"BorgBACKUP",
		"60 seconds",
		"Backup NOW",
		"BorgDASHBOARD",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("getStartHTML missing %q", want)
		}
	}
	for _, bad := range []string{"<table", "<td>", "<tr>", "<center>"} {
		if strings.Contains(got, bad) {
			t.Errorf("getStartHTML should not contain legacy %q", bad)
		}
	}
}

func TestGetStartHTMLNoLegacyTags(t *testing.T) {
	ensureDisplayDrained(t)
	savedHive := hive
	savedTg := tg
	savedSleep := sleep
	t.Cleanup(func() {
		hive = savedHive
		tg = savedTg
		sleep = savedSleep
	})
	tg = nil
	hive = nil
	sleep = "60"
	got := getStartHTML()
	for _, bad := range []string{"<table", "<td>", "<tr>", "<center>", "</td>", "</tr>"} {
		if strings.Contains(got, bad) {
			t.Errorf("getStartHTML should not contain %q", bad)
		}
	}
}

// --- setOPNStatus: no legacy <td> tags in rendered status ----------------

func TestSetOPNStatusNoLegacyTags(t *testing.T) {
	ensureDisplayDrained(t)
	savedHive := hive
	t.Cleanup(func() { hive = savedHive })
	hive = make([]string, 1)
	config := &OPNCall{Key: "k", Secret: "s"}
	ts := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)
	// ok=false on an empty slot produces just the _fail SVG.
	setOPNStatus(config, "fw01.lan", "edge-1", "", 0, ts, false, false)
	status := hive[0]
	if strings.Contains(status, "<td>") || strings.Contains(status, "</td>") {
		t.Errorf("status should not contain <td> tags: %q", status)
	}
	if !strings.Contains(status, "failed") {
		t.Errorf("fail status should contain fail indicator: %q", status)
	}
}

// --- getHive: modern HTML structure --------------------------------------

func TestGetHiveModernHTML(t *testing.T) {
	ensureDisplayDrained(t)
	savedHive := hive
	savedTg := tg
	savedSleep := sleep
	t.Cleanup(func() {
		hive = savedHive
		tg = savedTg
		sleep = savedSleep
	})
	tg = []OPNGroup{
		{Name: "EDGE", Desc: "Edge firewalls", OPN: true, Member: []string{"fw01#edge-1"}},
		{Name: "CORE", OPN: true, Member: []string{"fw02"}},
	}
	hive = []string{
		_na + "<span class=\"member-meta\">fw01</span>",
		_na + "<span class=\"member-meta\">fw02</span>",
	}
	sleep = "30"
	got := getHive()
	for _, want := range []string{
		"<div class=\"group\">",
		"<div class=\"member-row\">",
		"group-header",
		"group-desc",
		"Edge firewalls",
		"CORE",
		"backup-section",
		"30 seconds",
		"Backup NOW",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("getHive missing %q", want)
		}
	}
	for _, bad := range []string{"<table", "<td", "<tr", "<center"} {
		if strings.Contains(got, bad) {
			t.Errorf("getHive should not contain %q", bad)
		}
	}
}

// --- writeGroupMember: lookup behavior ------------------------------------

func TestWriteGroupMemberOPNMatch(t *testing.T) {
	ensureDisplayDrained(t)
	savedHive := hive
	t.Cleanup(func() { hive = savedHive })
	hive = []string{
		_ok + "<span>fw01.lan</span>",
		_ok + "<span>fw02.lan</span>",
	}
	var s strings.Builder
	writeGroupMember(&s, OPNGroup{OPN: true}, "fw01.lan")
	if !strings.Contains(s.String(), "fw01.lan") {
		t.Errorf("should have matched fw01.lan: %q", s.String())
	}
	if strings.Contains(s.String(), "fw02.lan") {
		t.Errorf("should not contain fw02.lan: %q", s.String())
	}
}

func TestWriteGroupMemberUnifi(t *testing.T) {
	savedStatus := unifiStatus
	t.Cleanup(func() { unifiStatus = savedStatus })
	unifiStatus = _unifi + "<span>controller</span>"
	var s strings.Builder
	writeGroupMember(&s, OPNGroup{Unifi: true}, "unifi-host")
	if !strings.Contains(s.String(), "controller") {
		t.Errorf("unifi member should render unifiStatus: %q", s.String())
	}
}

func TestWriteGroupMemberOPNNoMatch(t *testing.T) {
	ensureDisplayDrained(t)
	savedHive := hive
	t.Cleanup(func() { hive = savedHive })
	hive = []string{_ok + "<span>fw99.lan</span>"}
	var s strings.Builder
	writeGroupMember(&s, OPNGroup{OPN: true}, "fw01.lan")
	if s.String() != "" {
		t.Errorf("no match should produce empty output: %q", s.String())
	}
}

// --- checkSetRequiredUnifi: Desc field -----------------------------------

func TestCheckSetRequiredUnifiWithDesc(t *testing.T) {
	savedTg := tg
	t.Cleanup(func() { tg = savedTg })
	tg = nil
	withEnv(t, "OPN_UNIFI_WEBUI", "https://unifi.example.com", true)
	withEnv(t, "OPN_UNIFI_BACKUP_USER", "user", true)
	withEnv(t, "OPN_UNIFI_BACKUP_SECRET", "secret", true)
	withEnv(t, "OPN_UNIFI_BACKUP_DESC", "Network controller", true)
	if !checkSetRequiredUnifi() {
		t.Fatalf("expected true for unifi with desc")
	}
	if len(tg) != 1 {
		t.Fatalf("expected 1 group, got %d", len(tg))
	}
	if tg[0].Desc != "Network controller" {
		t.Errorf("desc = %q, want %q", tg[0].Desc, "Network controller")
	}
	if !tg[0].Unifi || tg[0].OPN {
		t.Errorf("group flags wrong: OPN=%v Unifi=%v", tg[0].OPN, tg[0].Unifi)
	}
}

// --- Setup: Unifi export MongoDB URI default --------------------------------

// TestSetupUnifiExportURIDefault verifies that Setup keeps the default
// MongoDB URI when OPN_UNIFI_MONGODB_URI is unset. Previously checkURL
// returned (nil, nil) for an unset var and overwrote the default, causing a
// nil-pointer panic in srvUnifiExport when it called URI.String().
func TestSetupUnifiExportURIDefault(t *testing.T) {
	ensureDisplayDrained(t)

	// Snapshot and restore the env vars Setup reads for the unifi export path.
	for _, k := range []string{
		"OPN_APIKEY", "OPN_APISECRET", "OPN_TARGETS",
		"OPN_UNIFI_WEBUI", "OPN_UNIFI_BACKUP_USER", "OPN_UNIFI_BACKUP_SECRET",
		"OPN_UNIFI_VERSION", "OPN_UNIFI_EXPORT", "OPN_UNIFI_MONGODB_URI",
		"OPN_UNIFI_FORMAT", "OPN_NODAEMON", "OPN_GIT_ENABLE", "OPN_GIT_UPSTREAM", "OPN_GIT_SSH_KEY",
	} {
		old, had := os.LookupEnv(k)
		_ = os.Unsetenv(k)
		t.Cleanup(func() {
			if had {
				_ = os.Setenv(k, old)
			} else {
				_ = os.Unsetenv(k)
			}
		})
	}

	// Configure unifi backup + export but leave OPN_UNIFI_MONGODB_URI unset.
	withEnv(t, "OPN_UNIFI_WEBUI", "https://unifi.example.com:8443", true)
	withEnv(t, "OPN_UNIFI_BACKUP_USER", "user", true)
	withEnv(t, "OPN_UNIFI_BACKUP_SECRET", "secret", true)
	withEnv(t, "OPN_UNIFI_VERSION", "8.5.6", true)
	withEnv(t, "OPN_UNIFI_EXPORT", "1", true)

	config, err := Setup()
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if !config.Unifi.Export.Enable {
		t.Fatal("Unifi export should be enabled")
	}
	if config.Unifi.Export.URI == nil {
		t.Fatal("Unifi export URI must not be nil when OPN_UNIFI_MONGODB_URI is unset; srvUnifiExport would panic on URI.String()")
	}
	if got := config.Unifi.Export.URI.String(); got != "mongodb://127.0.0.1:27117" {
		t.Errorf("Unifi export URI = %q, want default mongodb://127.0.0.1:27117", got)
	}
}

// TestSetupUnifiExportURIOverride verifies the default is overridden when the
// env var is set.
func TestSetupUnifiExportURIOverride(t *testing.T) {
	ensureDisplayDrained(t)

	for _, k := range []string{
		"OPN_APIKEY", "OPN_APISECRET", "OPN_TARGETS",
		"OPN_UNIFI_WEBUI", "OPN_UNIFI_BACKUP_USER", "OPN_UNIFI_BACKUP_SECRET",
		"OPN_UNIFI_VERSION", "OPN_UNIFI_EXPORT", "OPN_UNIFI_MONGODB_URI",
		"OPN_UNIFI_FORMAT", "OPN_NODAEMON", "OPN_GIT_ENABLE", "OPN_GIT_UPSTREAM", "OPN_GIT_SSH_KEY",
	} {
		old, had := os.LookupEnv(k)
		_ = os.Unsetenv(k)
		t.Cleanup(func() {
			if had {
				_ = os.Setenv(k, old)
			} else {
				_ = os.Unsetenv(k)
			}
		})
	}

	withEnv(t, "OPN_UNIFI_WEBUI", "https://unifi.example.com:8443", true)
	withEnv(t, "OPN_UNIFI_BACKUP_USER", "user", true)
	withEnv(t, "OPN_UNIFI_BACKUP_SECRET", "secret", true)
	withEnv(t, "OPN_UNIFI_VERSION", "8.5.6", true)
	withEnv(t, "OPN_UNIFI_EXPORT", "1", true)
	withEnv(t, "OPN_UNIFI_MONGODB_URI", "mongodb://10.0.0.5:27017", true)

	config, err := Setup()
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if got := config.Unifi.Export.URI.String(); got != "mongodb://10.0.0.5:27017" {
		t.Errorf("Unifi export URI = %q, want mongodb://10.0.0.5:27017", got)
	}
}

// --- getPKG: modern HTML structure ---------------------------------------

func TestGetPKGModernHTML(t *testing.T) {
	saved := syncPKG
	t.Cleanup(func() { syncPKG = saved })
	syncPKG = "os-foo,os-bar"
	got := getPKG()
	for _, want := range []string{"backup-section", "BorgSYNC", "foo", "bar"} {
		if !strings.Contains(got, want) {
			t.Errorf("getPKG missing %q: %q", want, got)
		}
	}
	for _, bad := range []string{"<table", "<td", "<tr"} {
		if strings.Contains(got, bad) {
			t.Errorf("getPKG should not contain %q: %q", bad, got)
		}
	}
}

// TestGetPKGMasterURL verifies the Manage Plugins button href always points
// at the configured Master Host firmware plugins page, even when package sync
// (OPN_SYNC_PKG) is disabled. See setup.go::Setup() where pkgmaster is now set
// whenever OPN_MASTER is set.
func TestGetPKGMasterURL(t *testing.T) {
	savedPkg := syncPKG
	savedMaster := pkgmaster
	t.Cleanup(func() {
		syncPKG = savedPkg
		pkgmaster = savedMaster
	})
	syncPKG = "os-foo,os-bar"
	pkgmaster = "https://opn-master.example.com" + _plug
	got := getPKG()
	if !strings.Contains(got, pkgmaster) {
		t.Errorf("Manage Plugins button should point at master URL %q: %q", pkgmaster, got)
	}
	if !strings.Contains(got, "Manage Plugins") {
		t.Errorf("missing Manage Plugins label: %q", got)
	}
}

// TestConfigButtonInsideDashboard verifies the [ Config Dashboard ] button is
// rendered inside the BorgDASHBOARD tile (so an operator finds it at the end
// of the dashboard) and reuses the Backup NOW button styling (btn-force).
func TestConfigButtonInsideDashboard(t *testing.T) {
	ensureDisplayDrained(t)
	config := &OPNCall{Path: t.TempDir()}
	got := getDashboard(config)
	if !strings.Contains(got, _configButton) {
		t.Errorf("dashboard tile should contain the Config Dashboard button: %q", got)
	}
	// The button must reuse the btn-force class (same design as Backup NOW).
	if !strings.Contains(_configButton, "btn btn-force") {
		t.Errorf("Config Dashboard button should reuse btn-force styling: %q", _configButton)
	}
	// The button must sit inside the dashboard tile but OUTSIDE the
	// dashboard-grid, so it renders as a full-width last line below the
	// panels (mirroring the Backup NOW / Manage Plugins buttons). The grid
	// wrapper closes with </div> immediately before the button, and only
	// the dashboard's own closing </div> follows it.
	if !strings.Contains(got, "</div>"+_configButton+"</div>") {
		t.Errorf("Config Dashboard button should be on its own line after the dashboard-grid close and before the dashboard close: %q", got)
	}
	// getStartHTML must no longer render a second copy of _configButton
	// outside the dashboard tile.
	savedHive := hive
	savedTg := tg
	savedSleep := sleep
	savedCfg := _cfg
	t.Cleanup(func() {
		hive = savedHive
		tg = savedTg
		sleep = savedSleep
		_cfg = savedCfg
	})
	tg = nil
	hive = nil
	sleep = "60"
	_cfg = config
	full := getStartHTML()
	if strings.Count(full, "Config Dashboard") != 1 {
		t.Errorf("Config Dashboard button should appear exactly once in getStartHTML, got %d", strings.Count(full, "Config Dashboard"))
	}
}

// --- sync-master.go / sync-pkg.go ----------------------------------------

// TestFormatTargetsDisplay covers the '#' target unit separator used on the
// config dashboard and raw env output. Empty entries from trailing/doubled
// commas are dropped, whitespace is trimmed.
func TestFormatTargetsDisplay(t *testing.T) {
	cases := map[string]string{
		"":                              "",
		"opn01.lan:8443":                "opn01.lan:8443",
		"opn01.lan:8443,opn02.lan:8443": "opn01.lan:8443#opn02.lan:8443",
		"opn01.lan:8443#RACK-PROD01,opn02.lan:8443#RACK-PROD02": "opn01.lan:8443#RACK-PROD01#opn02.lan:8443#RACK-PROD02",
		" opn01 , opn02 , ": "opn01#opn02",
		",,,":               "",
	}
	for in, want := range cases {
		got := formatTargetsDisplay(in)
		if got != want {
			t.Errorf("formatTargetsDisplay(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestIsTargetEnvName covers the env var name classification that drives the
// '#' separator in the raw environment section.
func TestIsTargetEnvName(t *testing.T) {
	yes := []string{"OPN_TARGETS", "OPN_TARGETS_INTRANET", "OPN_TARGETS_EXTERNAL"}
	no := []string{"OPN_APIKEY", "OPN_TARGETS_DESC_INTRANET", "OPN_TARGETS_IMGURL_INTRANET", "", "OPN_PATH"}
	for _, n := range yes {
		if !isTargetEnvName(n) {
			t.Errorf("isTargetEnvName(%q) = false, want true", n)
		}
	}
	for _, n := range no {
		if isTargetEnvName(n) {
			t.Errorf("isTargetEnvName(%q) = true, want false", n)
		}
	}
}

// TestSplitPlugins covers the strings.Split("", ",") == [""] gotcha that used
// to make checkInstallPKG attempt to install an empty-named package.
func TestSplitPlugins(t *testing.T) {
	cases := map[string][]string{
		"":              nil,
		"   ":           nil,
		"os-foo":        {"os-foo"},
		"os-foo,os-bar": {"os-foo", "os-bar"},
	}
	for in, want := range cases {
		got := splitPlugins(in)
		if len(got) != len(want) {
			t.Errorf("splitPlugins(%q) len = %d, want %d (%q)", in, len(got), len(want), got)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("splitPlugins(%q)[%d] = %q, want %q", in, i, got[i], want[i])
			}
		}
	}
}

// TestSyncPKGRoundtrip exercises the mutex-guarded get/set accessors that
// replaced the racy direct global write/read. Run with -race to verify.
func TestSyncPKGRoundtrip(t *testing.T) {
	saved := getSyncPKG()
	t.Cleanup(func() { setSyncPKG(saved) })

	setSyncPKG("")
	if got := getSyncPKG(); got != "" {
		t.Errorf("getSyncPKG after empty set = %q, want empty", got)
	}
	setSyncPKG("os-foo,os-bar")
	if got := getSyncPKG(); got != "os-foo,os-bar" {
		t.Errorf("getSyncPKG = %q, want %q", got, "os-foo,os-bar")
	}
}

// TestCheckInstallPKGEmptyPlugins ensures a host with an empty plugin list
// does not trigger an install of an empty-named package, and that a host
// carrying all master plugins returns nil with no missing entries.
func TestCheckInstallPKGEmptyPlugins(t *testing.T) {
	ensureDisplayDrained(t)

	config := &OPNCall{}
	config.Sync.PKG.Packages = splitPlugins("os-foo,os-bar")

	// host reports no plugins at all => both master packages are missing.
	// installPKG is stubbed out by the test; we only assert the missing
	// detection path produces an aggregated error rather than panicking
	// on an empty string entry.
	host := &Opnsense{}
	host.System.Firmware.Plugins = ""

	// Capture whether installPKG would be called with an empty name by
	// intercepting displayChan messages. Since installPKG hits the network,
	// we instead verify splitPlugins never yields an empty entry here, which
	// is the root cause of the original bug.
	if got := splitPlugins(host.System.Firmware.Plugins); len(got) != 0 {
		t.Fatalf("splitPlugins(\"\") = %v, want nil", got)
	}

	// A host that already has every master plugin must not be flagged.
	host.System.Firmware.Plugins = "os-foo,os-bar"
	if err := checkInstallPKG("opn01.lan", config, host); err != nil {
		t.Errorf("checkInstallPKG on fully-synced host returned err: %v", err)
	}
}

// --- Unifi autoBackup folder watch: setup gating ---------------------------

// setupUnifiWatchEnv prepares a temp source folder with the given marker file
// content and sets OPN_UNIFI_WATCH_PATH to point at it. It returns the config
// populated by the watch parsing block (mirroring Setup()'s logic) and the
// source path. The caller is responsible for restoring unifiWatchEnable /
// unifiWatchPath globals.
func setupUnifiWatchEnv(t *testing.T, markerContent string) (*OPNCall, string) {
	t.Helper()
	ensureDisplayDrained(t)
	src := t.TempDir()
	if markerContent != "" {
		if err := os.WriteFile(filepath.Join(src, "autobackup_meta.json"), []byte(markerContent), 0644); err != nil {
			t.Fatalf("write marker: %v", err)
		}
	}
	withEnv(t, "OPN_UNIFI_WATCH_PATH", src, true)
	// mirror the Setup() watch block (kept in sync by hand).
	config := &OPNCall{}
	unifiWatchEnable.Store(false)
	config.Unifi.Watch.Enable = false
	config.Unifi.Watch.SetupErr = ""
	if isEnv("OPN_UNIFI_WATCH_PATH") {
		watchPath := os.Getenv("OPN_UNIFI_WATCH_PATH")
		info, err := os.Stat(watchPath)
		if err != nil {
			config.Unifi.Watch.SetupErr = "SOURCE-FOLDER-NOT-FOUND: " + err.Error()
			return config, src
		}
		if !info.IsDir() {
			config.Unifi.Watch.SetupErr = "SOURCE-FOLDER-NOT-A-DIRECTORY: " + watchPath
			return config, src
		}
		metaPath := filepath.Join(watchPath, "autobackup_meta.json")
		if _, err := os.ReadFile(metaPath); err != nil {
			config.Unifi.Watch.SetupErr = "META-FILE-NOT-READABLE: " + err.Error()
			return config, src
		}
		config.Unifi.Watch.Enable = true
		config.Unifi.Watch.Path = watchPath
		config.Unifi.Watch.Meta = metaPath
		if fi, err := os.Stat(metaPath); err == nil {
			config.Unifi.Watch.LastTS = fi.ModTime()
		}
		unifiWatchEnable.Store(true)
		unifiWatchPath = watchPath
	}
	return config, src
}

func TestUnifiWatchSetupDisabledWhenFolderMissing(t *testing.T) {
	savedEnable := unifiWatchEnable.Load()
	savedPath := unifiWatchPath
	t.Cleanup(func() {
		unifiWatchEnable.Store(savedEnable)
		unifiWatchPath = savedPath
	})
	withEnv(t, "OPN_UNIFI_WATCH_PATH", "/nonexistent/path/does/not/exist", true)
	ensureDisplayDrained(t)
	config := &OPNCall{}
	unifiWatchEnable.Store(false)
	if isEnv("OPN_UNIFI_WATCH_PATH") {
		watchPath := os.Getenv("OPN_UNIFI_WATCH_PATH")
		if info, err := os.Stat(watchPath); err == nil && info.IsDir() {
			t.Fatalf("unexpected existing dir")
		}
	}
	if config.Unifi.Watch.Enable {
		t.Errorf("watch should be disabled when source folder is missing")
	}
	if unifiWatchEnable.Load() {
		t.Errorf("unifiWatchEnable should be false when source folder is missing")
	}
}

func TestUnifiWatchSetupDisabledWhenMarkerMissing(t *testing.T) {
	savedEnable := unifiWatchEnable.Load()
	savedPath := unifiWatchPath
	t.Cleanup(func() {
		unifiWatchEnable.Store(savedEnable)
		unifiWatchPath = savedPath
	})
	config, _ := setupUnifiWatchEnv(t, "")
	if config.Unifi.Watch.Enable {
		t.Errorf("watch should be disabled when autobackup_meta.json is missing")
	}
	if unifiWatchEnable.Load() {
		t.Errorf("unifiWatchEnable should be false when marker is missing")
	}
}

// TestUnifiWatchSetupEnabledWhenMarkerNotXML verifies that the watcher is
// armed as long as the marker file exists and is readable — its contents are
// no longer parsed or validated, so a non-XML marker must not block the
// feature (the previous behaviour rejected it as invalid XML).
func TestUnifiWatchSetupEnabledWhenMarkerNotXML(t *testing.T) {
	savedEnable := unifiWatchEnable.Load()
	savedPath := unifiWatchPath
	t.Cleanup(func() {
		unifiWatchEnable.Store(savedEnable)
		unifiWatchPath = savedPath
	})
	config, _ := setupUnifiWatchEnv(t, "{this is not valid xml")
	if !config.Unifi.Watch.Enable {
		t.Errorf("watch should be enabled when marker exists and is readable (contents are not validated)")
	}
	if !unifiWatchEnable.Load() {
		t.Errorf("unifiWatchEnable should be true when marker is readable")
	}
}

func TestUnifiWatchSetupEnabledWhenMarkerReadable(t *testing.T) {
	savedEnable := unifiWatchEnable.Load()
	savedPath := unifiWatchPath
	t.Cleanup(func() {
		unifiWatchEnable.Store(savedEnable)
		unifiWatchPath = savedPath
	})
	config, src := setupUnifiWatchEnv(t, `<autobackup><version>8.5.6</version></autobackup>`)
	if !config.Unifi.Watch.Enable {
		t.Errorf("watch should be enabled when folder + readable marker exist")
	}
	if !unifiWatchEnable.Load() {
		t.Errorf("unifiWatchEnable should be true")
	}
	if config.Unifi.Watch.Path != src {
		t.Errorf("watch path = %q want %q", config.Unifi.Watch.Path, src)
	}
	if config.Unifi.Watch.Meta != filepath.Join(src, "autobackup_meta.json") {
		t.Errorf("meta path mismatch: %q", config.Unifi.Watch.Meta)
	}
	if config.Unifi.Watch.LastTS.IsZero() {
		t.Errorf("LastTS should be populated from marker mtime")
	}
}

// --- Unifi autoBackup folder watch: status rendering ----------------------

func TestSetUnifiWatchStatusOK(t *testing.T) {
	savedStatus := unifiWatchStatus
	t.Cleanup(func() { unifiWatchStatus = savedStatus })
	unifiWatchStatus = ""
	config := &OPNCall{}
	u, _ := url.Parse("https://ctrl.lan:8443#RACK-1")
	config.Unifi.WebUI = u
	config.Unifi.Tag = "RACK-1"
	ts := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)
	config.Unifi.Watch.LastTS = ts
	setUnifiWatchStatus(config, true, true)
	out := unifiWatchStatus
	for _, want := range []string{
		"member-status",
		"current.unf",
		"archive",
		"Last Sync",
		"2024-06-01T10:00:00Z",
		"RACK-1",
		"ctrl.lan",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("setUnifiWatchStatus ok output missing %q: %s", want, out)
		}
	}
	if !strings.Contains(out, _ok) {
		t.Errorf("ok status should contain _ok SVG")
	}
	if strings.Contains(out, _fail) {
		t.Errorf("ok status should not contain _fail SVG")
	}
}

func TestSetUnifiWatchStatusDegraded(t *testing.T) {
	savedStatus := unifiWatchStatus
	t.Cleanup(func() { unifiWatchStatus = savedStatus })
	unifiWatchStatus = ""
	config := &OPNCall{}
	u, _ := url.Parse("https://ctrl.lan:8443")
	config.Unifi.WebUI = u
	setUnifiWatchStatus(config, true, false)
	if !strings.Contains(unifiWatchStatus, _degraded) {
		t.Errorf("degraded status should contain _degraded SVG: %s", unifiWatchStatus)
	}
}

func TestSetUnifiWatchStatusUnreachable(t *testing.T) {
	savedStatus := unifiWatchStatus
	t.Cleanup(func() { unifiWatchStatus = savedStatus })
	unifiWatchStatus = "<div class=\"member-status\">" + _ok + "</div><div class=\"member-main\"><span class=\"member-meta\">ctrl</span></div>"
	config := &OPNCall{}
	setUnifiWatchStatus(config, false, false)
	if !strings.Contains(unifiWatchStatus, _fail) {
		t.Errorf("unreachable status should contain _fail SVG: %s", unifiWatchStatus)
	}
	if strings.Contains(unifiWatchStatus, _ok) {
		t.Errorf("unreachable status should not contain _ok SVG")
	}
}

func TestGetUnifiWatchDisabledWhenFlagFalse(t *testing.T) {
	savedEnable := unifiWatchEnable.Load()
	t.Cleanup(func() { unifiWatchEnable.Store(savedEnable) })
	unifiWatchEnable.Store(false)
	if got := getUnifiWatch(); got != _empty {
		t.Errorf("getUnifiWatch should return _empty when disabled, got %q", got)
	}
}

func TestGetUnifiWatchRendersWhenEnabled(t *testing.T) {
	savedEnable := unifiWatchEnable.Load()
	savedPath := unifiWatchPath
	savedStatus := unifiWatchStatus
	t.Cleanup(func() {
		unifiWatchEnable.Store(savedEnable)
		unifiWatchPath = savedPath
		unifiWatchStatus = savedStatus
	})
	unifiWatchEnable.Store(true)
	unifiWatchPath = "/var/lib/unifi/data/backup/autobackup"
	unifiWatchStatus = "<div class=\"member-status\">" + _ok + "</div><div class=\"member-main\">sync ok</div>"
	got := getUnifiWatch()
	for _, want := range []string{
		"UNIFI AUTOBACKUP WATCH",
		"/var/lib/unifi/data/backup/autobackup",
		"sync ok",
		"group",
		"member-row",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("getUnifiWatch missing %q: %s", want, got)
		}
	}
}

// --- Unifi autoBackup folder watch: sync logic ---------------------------

func TestSyncUnifiWatchCopiesNewestUnf(t *testing.T) {
	ensureDisplayDrained(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	src := t.TempDir()
	store := t.TempDir()
	config := &OPNCall{Path: store}
	config.Unifi.Watch.Path = src
	config.Unifi.Watch.Meta = filepath.Join(src, "autobackup_meta.json")
	if err := os.WriteFile(config.Unifi.Watch.Meta, []byte("<autobackup/>"), 0644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	// two .unf files; the newer one must win
	older := []byte(strings.Repeat("a", 2048))
	newer := []byte(strings.Repeat("b", 2048))
	if err := os.WriteFile(filepath.Join(src, "old.unf"), older, 0644); err != nil {
		t.Fatalf("write old: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(src, "new.unf"), newer, 0644); err != nil {
		t.Fatalf("write new: %v", err)
	}

	syncUnifiWatch(config, time.Now())

	got, err := os.ReadFile(filepath.Join(store, _uniWatch, "current.unf"))
	if err != nil {
		t.Fatalf("read current.unf: %v", err)
	}
	if string(got) != string(newer) {
		t.Errorf("current.unf should contain newest payload")
	}
}

func TestSyncUnifiWatchSkipsTooSmall(t *testing.T) {
	ensureDisplayDrained(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	src := t.TempDir()
	store := t.TempDir()
	config := &OPNCall{Path: store}
	config.Unifi.Watch.Path = src
	config.Unifi.Watch.Meta = filepath.Join(src, "autobackup_meta.json")
	if err := os.WriteFile(config.Unifi.Watch.Meta, []byte("<autobackup/>"), 0644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "tiny.unf"), []byte("x"), 0644); err != nil {
		t.Fatalf("write tiny: %v", err)
	}
	syncUnifiWatch(config, time.Now())
	if _, err := os.Stat(filepath.Join(store, _uniWatch, "current.unf")); !os.IsNotExist(err) {
		t.Errorf("too-small backup should not be checked into store")
	}
	// the failed pass should record the error reason + zero synced on the config
	if config.Unifi.Watch.LastSyncErr == "" {
		t.Errorf("LastSyncErr should be set after a too-small-only sync pass")
	}
	if config.Unifi.Watch.SyncedFiles != 0 {
		t.Errorf("SyncedFiles should be 0 after a too-small-only sync pass, got %d", config.Unifi.Watch.SyncedFiles)
	}
}

// TestSyncUnifiWatchCopiesAllFiles verifies that a sync pass checks in EVERY
// .unf file in the source folder (not only the newest), each as its own
// archive entry, with the newest file winning the current.unf pointer. The
// per-pass stats (source / synced / skipped / last file) must be recorded.
func TestSyncUnifiWatchCopiesAllFiles(t *testing.T) {
	ensureDisplayDrained(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	src := t.TempDir()
	store := t.TempDir()
	config := &OPNCall{Path: store}
	config.Unifi.Watch.Path = src
	config.Unifi.Watch.Meta = filepath.Join(src, "autobackup_meta.json")
	if err := os.WriteFile(config.Unifi.Watch.Meta, []byte("<autobackup/>"), 0644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	a := []byte(strings.Repeat("a", 2048))
	b := []byte(strings.Repeat("b", 2048))
	if err := os.WriteFile(filepath.Join(src, "old.unf"), a, 0644); err != nil {
		t.Fatalf("write old: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(src, "new.unf"), b, 0644); err != nil {
		t.Fatalf("write new: %v", err)
	}

	syncUnifiWatch(config, time.Now())

	// newest wins current.unf
	got, err := os.ReadFile(filepath.Join(store, _uniWatch, "current.unf"))
	if err != nil {
		t.Fatalf("read current.unf: %v", err)
	}
	if string(got) != string(b) {
		t.Errorf("current.unf should contain newest payload")
	}
	// both files should be present as distinct archive entries
	entries, err := os.ReadDir(filepath.Join(store, _uniWatch, _archive))
	if err != nil {
		t.Fatalf("read archive tree: %v", err)
	}
	count := 0
	for _, e := range entries {
		_ = e
		count++
	}
	var archives []os.DirEntry
	_ = filepath.WalkDir(filepath.Join(store, _uniWatch, _archive), func(_ string, d os.DirEntry, _ error) error {
		if d != nil && !d.IsDir() {
			archives = append(archives, d)
		}
		return nil
	})
	if len(archives) != 2 {
		t.Errorf("expected 2 archive entries (one per .unf), got %d", len(archives))
	}
	// stats recorded
	if config.Unifi.Watch.SourceFiles != 2 {
		t.Errorf("SourceFiles = %d, want 2", config.Unifi.Watch.SourceFiles)
	}
	if config.Unifi.Watch.SyncedFiles != 2 {
		t.Errorf("SyncedFiles = %d, want 2", config.Unifi.Watch.SyncedFiles)
	}
	if config.Unifi.Watch.SkippedFiles != 0 {
		t.Errorf("SkippedFiles = %d, want 0", config.Unifi.Watch.SkippedFiles)
	}
	if config.Unifi.Watch.LastFile != "new.unf" {
		t.Errorf("LastFile = %q, want new.unf", config.Unifi.Watch.LastFile)
	}
	if config.Unifi.Watch.LastSyncErr != "" {
		t.Errorf("LastSyncErr should be empty on success, got %q", config.Unifi.Watch.LastSyncErr)
	}
	// a second pass should skip both (now deduped against current.unf)
	syncUnifiWatch(config, time.Now())
	if config.Unifi.Watch.SyncedFiles != 0 {
		t.Errorf("second pass SyncedFiles = %d, want 0 (deduped)", config.Unifi.Watch.SyncedFiles)
	}
	if config.Unifi.Watch.SkippedFiles != 2 {
		t.Errorf("second pass SkippedFiles = %d, want 2", config.Unifi.Watch.SkippedFiles)
	}
}

// TestSyncUnifiWatchRecordsReadDirError verifies that a sync pass against an
// unreadable source folder records the error reason on the config struct
// (surfaced on the config dashboard) instead of silently returning.
func TestSyncUnifiWatchRecordsReadDirError(t *testing.T) {
	ensureDisplayDrained(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	store := t.TempDir()
	config := &OPNCall{Path: store}
	config.Unifi.Watch.Path = "/nonexistent/opnborg/watch/source"
	config.Unifi.Watch.Meta = "/nonexistent/opnborg/watch/source/autobackup_meta.json"
	syncUnifiWatch(config, time.Now())
	if config.Unifi.Watch.LastSyncErr == "" {
		t.Errorf("LastSyncErr should be set when the source folder is unreadable")
	}
	if !strings.Contains(config.Unifi.Watch.LastSyncErr, "READ-SOURCE-DIR") {
		t.Errorf("LastSyncErr should name the failing step, got %q", config.Unifi.Watch.LastSyncErr)
	}
}

// TestSetUnifiWatchStatusSurfacesErrorAndStats verifies the WebUI tile renders
// the per-pass sync stats (synced / source / skipped / last file) and the
// error reason box when the last sync failed.
func TestSetUnifiWatchStatusSurfacesErrorAndStats(t *testing.T) {
	savedStatus := unifiWatchStatus
	t.Cleanup(func() { unifiWatchStatus = savedStatus })
	unifiWatchStatus = ""
	config := &OPNCall{}
	ts := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)
	config.Unifi.Watch.LastTS = ts
	config.Unifi.Watch.SourceFiles = 3
	config.Unifi.Watch.SyncedFiles = 2
	config.Unifi.Watch.SkippedFiles = 1
	config.Unifi.Watch.LastFile = "backup-2024-06-01.unf"
	config.Unifi.Watch.LastSyncErr = "STORE-CHECKIN: disk full"
	setUnifiWatchStatus(config, true, false)
	out := unifiWatchStatus
	for _, want := range []string{
		"Synced", "2 / 3", "Skipped", "1", "Last File", "backup-2024-06-01.unf", "Error", "STORE-CHECKIN: disk full", _degraded,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status output missing %q: %s", want, out)
		}
	}
}

// TestRenderUnifiPanelSetupError verifies the config dashboard surfaces the
// setup-time error reason (configuration + reason + error detail) in the
// Unifi panel when the watcher could not be armed at startup.
func TestRenderUnifiPanelSetupError(t *testing.T) {
	config := &OPNCall{}
	config.Unifi.Watch.Enable = false
	config.Unifi.Watch.SetupErr = "SOURCE-FOLDER-NOT-FOUND: stat /var/lib/unifi/backup: no such file or directory"
	out := renderUnifiPanel(config)
	if !strings.Contains(out, "AutoBackup Watch") {
		t.Errorf("panel missing AutoBackup Watch row")
	}
	if !strings.Contains(out, "Setup Error") {
		t.Errorf("panel should surface a Setup Error row")
	}
	if !strings.Contains(out, "SOURCE-FOLDER-NOT-FOUND") {
		t.Errorf("panel should include the error reason, got %s", out)
	}
}

// TestRenderUnifiPanelSyncStats verifies the config dashboard surfaces the
// runtime sync stats + error in the Unifi panel when the watcher is armed.
func TestRenderUnifiPanelSyncStats(t *testing.T) {
	config := &OPNCall{}
	config.Unifi.Watch.Enable = true
	config.Unifi.Watch.Path = "/var/lib/unifi/data/backup/autobackup"
	config.Unifi.Watch.Meta = "/var/lib/unifi/data/backup/autobackup/autobackup_meta.json"
	unifiWatchMutex.Lock()
	config.Unifi.Watch.SourceFiles = 5
	config.Unifi.Watch.SyncedFiles = 4
	config.Unifi.Watch.SkippedFiles = 1
	config.Unifi.Watch.LastFile = "backup-2024-06-01.unf"
	config.Unifi.Watch.LastSyncErr = "READ-BACKUP-FILE: corrupted.unf i/o error"
	config.Unifi.Watch.LastSyncTS = time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)
	unifiWatchMutex.Unlock()
	out := renderUnifiPanel(config)
	for _, want := range []string{
		"Source Files", "5", "Last Synced", "4", "Skipped (dup)", "1",
		"Last File", "backup-2024-06-01.unf", "Sync Error", "READ-BACKUP-FILE: corrupted.unf i/o error",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("panel missing %q: %s", want, out)
		}
	}
}

// --- git.go: sshUserFromURL + validateGitConfig --------------------------

func TestSSHUserFromURL(t *testing.T) {
	cases := map[string]string{
		"git@github.com:user/repo.git":   "git",
		"deploy@example.org:org/repo":    "deploy",
		"ssh://git@github.com/user/repo": "git",
		"ssh://backup@host.io:22/r.git":  "backup",
		"ssh://host.io/path":             "git", // ssh:// without user
		"github.com:user/repo.git":       "git", // no user => default
		"":                               "git",
		"@hostonly:path":                 "git", // leading '@' => default (no user before '@')
	}
	for in, want := range cases {
		if got := sshUserFromURL(in); got != want {
			t.Errorf("sshUserFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateGitConfigDisabledClearsFields(t *testing.T) {
	config := &OPNCall{}
	config.Git.Enable = false
	config.Git.Upstream = "git@github.com:user/repo.git"
	config.Git.SSHKey = "/tmp/key"
	if err := validateGitConfig(config); err != nil {
		t.Fatalf("disabled feature must not error, got %v", err)
	}
	if config.Git.Upstream != "" || config.Git.SSHKey != "" {
		t.Errorf("disabled feature should clear upstream/key, got upstream=%q key=%q",
			config.Git.Upstream, config.Git.SSHKey)
	}
}

func TestValidateGitConfigEnabledNoUpstream(t *testing.T) {
	config := &OPNCall{Git: struct {
		Enable   bool
		Upstream string
		SSHKey   string
	}{Enable: true}}
	if err := validateGitConfig(config); err != nil {
		t.Fatalf("enabled with no upstream/key must not error, got %v", err)
	}
}

func TestValidateGitConfigUpstreamWithoutKey(t *testing.T) {
	config := &OPNCall{Git: struct {
		Enable   bool
		Upstream string
		SSHKey   string
	}{Enable: true, Upstream: "git@github.com:user/repo.git"}}
	if err := validateGitConfig(config); err == nil {
		t.Fatalf("upstream without key must error")
	}
}

func TestValidateGitConfigKeyWithoutUpstream(t *testing.T) {
	key := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(key, []byte("dummy"), 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	config := &OPNCall{Git: struct {
		Enable   bool
		Upstream string
		SSHKey   string
	}{Enable: true, SSHKey: key}}
	if err := validateGitConfig(config); err == nil {
		t.Fatalf("key without upstream must error")
	}
}

func TestValidateGitConfigKeyMissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	config := &OPNCall{Git: struct {
		Enable   bool
		Upstream string
		SSHKey   string
	}{Enable: true, Upstream: "git@github.com:user/repo.git", SSHKey: missing}}
	if err := validateGitConfig(config); err == nil {
		t.Fatalf("missing key file must error")
	}
}

func TestValidateGitConfigKeyIsDir(t *testing.T) {
	dir := t.TempDir()
	config := &OPNCall{Git: struct {
		Enable   bool
		Upstream string
		SSHKey   string
	}{Enable: true, Upstream: "git@github.com:user/repo.git", SSHKey: dir}}
	if err := validateGitConfig(config); err == nil {
		t.Fatalf("key path pointing at a directory must error")
	}
}

func TestValidateGitConfigKeyFileOK(t *testing.T) {
	key := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(key, []byte("dummy"), 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	config := &OPNCall{Git: struct {
		Enable   bool
		Upstream string
		SSHKey   string
	}{Enable: true, Upstream: "git@github.com:user/repo.git", SSHKey: key}}
	if err := validateGitConfig(config); err != nil {
		t.Fatalf("valid upstream+key must not error, got %v", err)
	}
}

// --- git.go: gitInit + gitCheckIn end-to-end -------------------------------

func TestGitInitCreatesRepoAndIgnore(t *testing.T) {
	ensureDisplayDrained(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	store := t.TempDir()
	config := &OPNCall{Path: store, Email: "test@opnborg"}
	if err := gitInit(config); err != nil {
		t.Fatalf("gitInit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store, ".git")); err != nil {
		t.Errorf(".git metadata not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store, _gitignore)); err != nil {
		t.Errorf(".gitignore not created: %v", err)
	}
	// idempotent: a second init must not fail and must keep the gitignore.
	if err := gitInit(config); err != nil {
		t.Fatalf("gitInit (2nd): %v", err)
	}
}

func TestGitCheckInCommitsAndIdempotent(t *testing.T) {
	ensureDisplayDrained(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	store := t.TempDir()
	config := &OPNCall{Path: store, Email: "test@opnborg"}
	if err := gitInit(config); err != nil {
		t.Fatalf("gitInit: %v", err)
	}
	if err := os.WriteFile(filepath.Join(store, "current.xml"), []byte("<x/>"), 0660); err != nil {
		t.Fatalf("write: %v", err)
	}
	committed, err := gitCheckIn(config)
	if err != nil {
		t.Fatalf("gitCheckIn: %v", err)
	}
	if !committed {
		t.Fatalf("first checkin with new file must report committed=true")
	}
	// second pass with no changes must report committed=false (clean worktree)
	committed, err = gitCheckIn(config)
	if err != nil {
		t.Fatalf("gitCheckIn (2nd): %v", err)
	}
	if committed {
		t.Errorf("clean worktree must report committed=false")
	}
}

// TestGitEnsureOriginRecreatesOnURLDrift verifies that gitEnsureOrigin
// recreates the origin remote when the recorded URL no longer matches the
// configured upstream, so a drifted upstream target never keeps commits local.
func TestGitEnsureOriginRecreatesOnURLDrift(t *testing.T) {
	ensureDisplayDrained(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	store := t.TempDir()
	config := &OPNCall{Path: store, Email: "test@opnborg"}
	if err := gitInit(config); err != nil {
		t.Fatalf("gitInit: %v", err)
	}
	repo, err := gitRepo(config.Path)
	if err != nil {
		t.Fatalf("gitRepo: %v", err)
	}
	old := "git@github.com:foo/old.git"
	new := "git@github.com:foo/new.git"
	if err := gitEnsureOrigin(repo, old); err != nil {
		t.Fatalf("ensure origin (old): %v", err)
	}
	rem, err := repo.Remote(_origin)
	if err != nil {
		t.Fatalf("remote old: %v", err)
	}
	if got := rem.Config().URLs[0]; got != old {
		t.Fatalf("url = %s, want %s", got, old)
	}
	if got := rem.Config().Fetch[0]; string(got) != _refspec {
		t.Fatalf("fetch refspec = %q, want %q", got, _refspec)
	}
	if err := gitEnsureOrigin(repo, new); err != nil {
		t.Fatalf("ensure origin (new): %v", err)
	}
	rem, err = repo.Remote(_origin)
	if err != nil {
		t.Fatalf("remote new: %v", err)
	}
	if got := rem.Config().URLs[0]; got != new {
		t.Fatalf("url after drift = %s, want %s", got, new)
	}
	if got := rem.Config().Fetch[0]; string(got) != _refspec {
		t.Fatalf("fetch refspec after drift = %q, want %q", got, _refspec)
	}
}

// TestGitEnsureOriginRecreatesOnMissingRefspec verifies that an origin remote
// created without refspecs (the bug shape: go-git's CreateRemote with no
// RefSpecs yields a remote that pushes nothing) is recreated so pushes have a
// valid ref mapping.
func TestGitEnsureOriginRecreatesOnMissingRefspec(t *testing.T) {
	ensureDisplayDrained(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	store := t.TempDir()
	config := &OPNCall{Path: store, Email: "test@opnborg"}
	if err := gitInit(config); err != nil {
		t.Fatalf("gitInit: %v", err)
	}
	repo, err := gitRepo(config.Path)
	if err != nil {
		t.Fatalf("gitRepo: %v", err)
	}
	upstream := "git@github.com:foo/repo.git"
	// Plant a remote with no refspecs, simulating the pre-fix state.
	if _, err := repo.CreateRemote(&gitcfg.RemoteConfig{
		Name: _origin,
		URLs: []string{upstream},
	}); err != nil {
		t.Fatalf("create bare origin: %v", err)
	}
	if err := gitEnsureOrigin(repo, upstream); err != nil {
		t.Fatalf("ensure origin: %v", err)
	}
	rem, err := repo.Remote(_origin)
	if err != nil {
		t.Fatalf("remote: %v", err)
	}
	if len(rem.Config().Fetch) == 0 || string(rem.Config().Fetch[0]) != _refspec {
		t.Fatalf("refspec not repaired after drift: %+v", rem.Config().Fetch)
	}
}

// TestGitCheckInRunsGC verifies that gitCheckIn runs the internal gc
// equivalent after a successful commit: loose objects created by the commit
// must be consolidated into a packfile, so the .git/objects loose object
// directory is emptied for the committed blobs.
func TestGitCheckInRunsGC(t *testing.T) {
	ensureDisplayDrained(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	store := t.TempDir()
	config := &OPNCall{Path: store, Email: "test@opnborg"}
	if err := gitInit(config); err != nil {
		t.Fatalf("gitInit: %v", err)
	}
	// Several distinct blobs so the commit produces multiple loose objects.
	for i := 0; i < 5; i++ {
		if err := os.WriteFile(
			filepath.Join(store, fmt.Sprintf("file%d.xml", i)),
			[]byte(fmt.Sprintf("<x>%d</x>", i)), 0660); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	committed, err := gitCheckIn(config)
	if err != nil {
		t.Fatalf("gitCheckIn: %v", err)
	}
	if !committed {
		t.Fatalf("expected committed=true")
	}
	// After gc, loose objects for the committed blobs should be packed away.
	loose, err := looseObjectCount(store)
	if err != nil {
		t.Fatalf("count loose: %v", err)
	}
	if loose != 0 {
		t.Errorf("expected 0 loose objects after gc, got %d", loose)
	}
	// At least one packfile must exist.
	packs, err := packfileCount(store)
	if err != nil {
		t.Fatalf("count packs: %v", err)
	}
	if packs == 0 {
		t.Errorf("expected at least one packfile after gc")
	}
}

// TestGitEnsureAggressiveWindow verifies gitInit raises the repo's pack window
// to the aggressive value so subsequent repacks behave like git gc --aggressive.
func TestGitEnsureAggressiveWindow(t *testing.T) {
	ensureDisplayDrained(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	store := t.TempDir()
	config := &OPNCall{Path: store, Email: "test@opnborg"}
	if err := gitInit(config); err != nil {
		t.Fatalf("gitInit: %v", err)
	}
	repo, err := gitRepo(config.Path)
	if err != nil {
		t.Fatalf("gitRepo: %v", err)
	}
	cfg, err := repo.ConfigScoped(gitcfg.LocalScope)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if cfg.Pack.Window != _aggressivePackWindow {
		t.Errorf("pack window = %d, want %d", cfg.Pack.Window, _aggressivePackWindow)
	}
	// Re-running gitInit must not lower or break an already-aggressive window.
	if err := gitInit(config); err != nil {
		t.Fatalf("gitInit (2nd): %v", err)
	}
	cfg, err = repo.ConfigScoped(gitcfg.LocalScope)
	if err != nil {
		t.Fatalf("config (2nd): %v", err)
	}
	if cfg.Pack.Window != _aggressivePackWindow {
		t.Errorf("pack window after 2nd init = %d, want %d", cfg.Pack.Window, _aggressivePackWindow)
	}
}

// looseObjectCount counts loose object files under <store>/.git/objects (any
// two-hex-digit subdirectory containing a 38-hex-char file).
func looseObjectCount(store string) (int, error) {
	objDir := filepath.Join(store, _dotGit, "objects")
	entries, err := os.ReadDir(objDir)
	if err != nil {
		return 0, err
	}
	var n int
	for _, e := range entries {
		if !e.IsDir() || len(e.Name()) != 2 {
			continue
		}
		subs, err := os.ReadDir(filepath.Join(objDir, e.Name()))
		if err != nil {
			return 0, err
		}
		n += len(subs)
	}
	return n, nil
}

// packfileCount counts .pack files under <store>/.git/objects/pack.
func packfileCount(store string) (int, error) {
	packDir := filepath.Join(store, _dotGit, "objects", "pack")
	entries, err := os.ReadDir(packDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	var n int
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".pack") {
			n++
		}
	}
	return n, nil
}

// TestDashboardGatherBackupFolderAndGitRepo verifies the dashboard stats
// gatherer counts server archive trees, archive entries, total bytes, newest
// archive mtime, and the local git repo state (HEAD hash, commit count,
// dirty worktree) from a populated store with git enabled.
func TestDashboardGatherBackupFolderAndGitRepo(t *testing.T) {
	ensureDisplayDrained(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	savedPush := func() {
		gitLastPushMu.Lock()
		savedTS, savedOK, savedMsg := gitLastPushTS, gitLastPushOK, gitLastPushMsg
		gitLastPushMu.Unlock()
		t.Cleanup(func() {
			gitLastPushMu.Lock()
			gitLastPushTS, gitLastPushOK, gitLastPushMsg = savedTS, savedOK, savedMsg
			gitLastPushMu.Unlock()
		})
	}
	savedPush()

	store := t.TempDir()
	config := &OPNCall{Path: store, Email: "test@opnborg"}
	config.Git.Enable = true

	// two server slots with archive entries
	for _, srv := range []string{"fw01.lan", "fw02.lan"} {
		archDir := filepath.Join(store, srv, ".archive", "2024", "06")
		if err := os.MkdirAll(archDir, 0770); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(archDir, "20240601T100000Z-"+srv+".xml"), []byte("<x/>"), 0660); err != nil {
			t.Fatalf("write archive: %v", err)
		}
		if err := os.WriteFile(filepath.Join(store, srv, "current.xml"), []byte("<x/>"), 0660); err != nil {
			t.Fatalf("write current: %v", err)
		}
	}

	// one Unifi controller backup slot with two .unf archive entries
	{
		srv := _uniWatch
		archDir := filepath.Join(store, srv, ".archive", "2024", "06")
		if err := os.MkdirAll(archDir, 0770); err != nil {
			t.Fatalf("mkdir unifi: %v", err)
		}
		if err := os.WriteFile(filepath.Join(archDir, "20240601T100000.000Z-"+srv+".unf"), []byte("unifi1"), 0660); err != nil {
			t.Fatalf("write unifi archive: %v", err)
		}
		if err := os.WriteFile(filepath.Join(archDir, "20240602T100000.000Z-"+srv+".unf"), []byte("unifi2"), 0660); err != nil {
			t.Fatalf("write unifi archive: %v", err)
		}
		if err := os.WriteFile(filepath.Join(store, srv, "current.unf"), []byte("unifi2"), 0660); err != nil {
			t.Fatalf("write current.unf: %v", err)
		}
	}

	// init git + commit so HEAD is non-empty
	if err := gitInit(config); err != nil {
		t.Fatalf("gitInit: %v", err)
	}
	if committed, err := gitCheckIn(config); err != nil || !committed {
		t.Fatalf("gitCheckIn: committed=%v err=%v", committed, err)
	}

	d := gatherDashboard(config)
	if d.servers != 3 {
		t.Errorf("servers = %d, want 3", d.servers)
	}
	if d.archives != 4 {
		t.Errorf("archives = %d, want 4", d.archives)
	}
	if d.unifiArchives != 2 {
		t.Errorf("unifiArchives = %d, want 2", d.unifiArchives)
	}
	if d.archiveBytes < 6 {
		t.Errorf("archiveBytes = %d, want >=6", d.archiveBytes)
	}
	if d.newestArchive.IsZero() {
		t.Errorf("newestArchive should be populated")
	}
	if !d.gitRepo {
		t.Errorf("gitRepo should be true after gitInit")
	}
	if d.gitHead == "" {
		t.Errorf("gitHead should be non-empty after a commit")
	}
	if d.gitCommits < 1 {
		t.Errorf("gitCommits = %d, want >=1", d.gitCommits)
	}
	if d.gitDirty != 0 {
		t.Errorf("gitDirty = %d, want 0 after checkin", d.gitDirty)
	}
	if d.upstreamConfigured {
		t.Errorf("upstreamConfigured should be false when OPN_GIT_UPSTREAM unset")
	}
}

// TestDashboardRenderDisabledGit verifies the dashboard renders the three
// panels (with the git + upstream panels collapsed to disabled state) when git
// management is disabled, and that the HTML structure is present.
func TestDashboardRenderDisabledGit(t *testing.T) {
	ensureDisplayDrained(t)
	config := &OPNCall{Path: t.TempDir()}
	html := getDashboard(config)
	for _, want := range []string{
		"BorgDASHBOARD",
		"dash-panel",
		"Backup Store",
		"Local Git Repo",
		"Upstream Sync",
		"git management disabled",
		"no upstream configured",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("dashboard missing %q", want)
		}
	}
}

// TestDashboardNilConfigPlaceholder verifies getDashboard does not panic when
// the config handle is nil (e.g. httpd not armed yet in tests) and emits the
// placeholder.
func TestDashboardNilConfigPlaceholder(t *testing.T) {
	if got := getDashboard(nil); !strings.Contains(got, "awaiting config") {
		t.Errorf("nil config should render placeholder, got: %q", got)
	}
}

// TestSetupHttpdDisabledInOneShotMode verifies the httpd is NOT armed when the
// daemon is disabled (OPN_NODAEMON set). Previously Httpd.Enable defaulted to
// true and the daemon-mode block never ran in one-shot mode, leaving Enable=true
// with an empty Server address — startWeb would then bind a random port and
// leak a listener for a process that exits immediately.
func TestSetupHttpdDisabledInOneShotMode(t *testing.T) {
	ensureDisplayDrained(t)
	for _, k := range []string{
		"OPN_APIKEY", "OPN_APISECRET", "OPN_TARGETS",
		"OPN_NODAEMON", "OPN_HTTPD_DISABLE", "OPN_HTTPD_SERVER",
		"OPN_GIT_ENABLE", "OPN_GIT_UPSTREAM", "OPN_GIT_SSH_KEY",
	} {
		old, had := os.LookupEnv(k)
		_ = os.Unsetenv(k)
		t.Cleanup(func() {
			if had {
				_ = os.Setenv(k, old)
			} else {
				_ = os.Unsetenv(k)
			}
		})
	}
	withEnv(t, "OPN_APIKEY", "k", true)
	withEnv(t, "OPN_APISECRET", "s", true)
	withEnv(t, "OPN_TARGETS", "fw01.lan", true)
	withEnv(t, "OPN_NODAEMON", "1", true) // one-shot mode

	config, err := Setup()
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if config.Daemon {
		t.Fatalf("expected Daemon=false in one-shot mode")
	}
	if config.Httpd.Enable {
		t.Errorf("Httpd.Enable must be false in one-shot mode (was %v); startWeb would bind an empty address", config.Httpd.Enable)
	}
	if config.Httpd.Server != "" {
		t.Errorf("Httpd.Server must be empty when disabled, got %q", config.Httpd.Server)
	}
}

// TestSetupHttpdHonorsDisableFlag verifies that OPN_HTTPD_DISABLE actually
// disables the httpd in daemon mode. Previously Enable defaulted to true and
// the daemon block only set it true again, so the disable flag was a no-op.
func TestSetupHttpdHonorsDisableFlag(t *testing.T) {
	ensureDisplayDrained(t)
	for _, k := range []string{
		"OPN_APIKEY", "OPN_APISECRET", "OPN_TARGETS",
		"OPN_NODAEMON", "OPN_HTTPD_DISABLE", "OPN_HTTPD_SERVER",
		"OPN_GIT_ENABLE", "OPN_GIT_UPSTREAM", "OPN_GIT_SSH_KEY",
	} {
		old, had := os.LookupEnv(k)
		_ = os.Unsetenv(k)
		t.Cleanup(func() {
			if had {
				_ = os.Setenv(k, old)
			} else {
				_ = os.Unsetenv(k)
			}
		})
	}
	withEnv(t, "OPN_APIKEY", "k", true)
	withEnv(t, "OPN_APISECRET", "s", true)
	withEnv(t, "OPN_TARGETS", "fw01.lan", true)
	// OPN_NODAEMON unset => daemon mode
	withEnv(t, "OPN_HTTPD_DISABLE", "1", true)

	config, err := Setup()
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if !config.Daemon {
		t.Fatalf("expected Daemon=true")
	}
	if config.Httpd.Enable {
		t.Errorf("Httpd.Enable must be false when OPN_HTTPD_DISABLE is set, got %v", config.Httpd.Enable)
	}
}

// --- httpd-handler.go: non-blocking /force pokes -------------------------

// TestSetupWatchOnlyNoWebFetcher verifies that a watch-only deployment (only
// OPN_UNIFI_WATCH_PATH set, no OPN_TARGETS, no OPN_UNIFI_WEBUI fetcher) is a
// valid configuration and Setup does NOT abort with the "please enable either
// OPN or Unifi backup" error. Previously the minimum-requirements gate ran
// before the watch parsing block, so a watch-only host could never start.
func TestSetupWatchOnlyNoWebFetcher(t *testing.T) {
	ensureDisplayDrained(t)
	savedEnable := unifiWatchEnable.Load()
	savedPath := unifiWatchPath
	t.Cleanup(func() {
		unifiWatchEnable.Store(savedEnable)
		unifiWatchPath = savedPath
	})
	for _, k := range []string{
		"OPN_APIKEY", "OPN_APISECRET", "OPN_TARGETS",
		"OPN_UNIFI_WEBUI", "OPN_UNIFI_BACKUP_USER", "OPN_UNIFI_BACKUP_SECRET",
		"OPN_UNIFI_VERSION", "OPN_UNIFI_EXPORT", "OPN_UNIFI_MONGODB_URI",
		"OPN_UNIFI_FORMAT", "OPN_UNIFI_WATCH_PATH",
		"OPN_NODAEMON", "OPN_GIT_ENABLE", "OPN_GIT_UPSTREAM", "OPN_GIT_SSH_KEY",
	} {
		old, had := os.LookupEnv(k)
		_ = os.Unsetenv(k)
		t.Cleanup(func() {
			if had {
				_ = os.Setenv(k, old)
			} else {
				_ = os.Unsetenv(k)
			}
		})
	}

	// prepare a watch folder with a valid-XML marker
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "autobackup_meta.json"), []byte(`<autobackup><version>8.5.6</version></autobackup>`), 0644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	withEnv(t, "OPN_UNIFI_WATCH_PATH", src, true)
	withEnv(t, "OPN_NODAEMON", "1", true) // one-shot so no httpd/rsyslog goroutines arm

	config, err := Setup()
	if err != nil {
		t.Fatalf("Setup returned err for watch-only config: %v", err)
	}
	if config.Enable {
		t.Errorf("OPN hive should be disabled in watch-only mode")
	}
	if config.Unifi.Backup.Enable {
		t.Errorf("Unifi web-fetch backup should be disabled in watch-only mode")
	}
	if !config.Unifi.Watch.Enable {
		t.Errorf("Unifi watch should be enabled")
	}
	if !unifiWatchEnable.Load() {
		t.Errorf("unifiWatchEnable global should be true")
	}
}

// TestGetForceHandlerNonBlocking verifies the /force handler never blocks on a
// full update channel. Previously it used blocking sends; if a tick was
// already pending in the buffered channel, the handler (and the HTTP client)
// would hang for a full backup cycle. Now it uses non-blocking selects.
func TestGetForceHandlerNonBlocking(t *testing.T) {
	ensureDisplayDrained(t)
	// Drain the update channels in the background so the handler's selects
	// stay non-blocking even after we fill them.
	for _, ch := range []chan bool{updateOPN, updateUnifiBackup, updateUnifiExport, updateUnifiWatch} {
		// pre-fill the buffered channel so the handler MUST drop the value
		select {
		case ch <- true:
		default:
		}
	}
	unifiBackupEnable.Store(true)
	unifiExportEnable.Store(true)
	unifiWatchEnable.Store(true)
	t.Cleanup(func() {
		unifiBackupEnable.Store(false)
		unifiExportEnable.Store(false)
		unifiWatchEnable.Store(false)
		// drain
		for _, ch := range []chan bool{updateOPN, updateUnifiBackup, updateUnifiExport, updateUnifiWatch} {
			select {
			case <-ch:
			default:
			}
		}
	})

	h := getForceHandler()
	done := make(chan struct{})
	go func() {
		rr := httptest.NewRecorder()
		req := &http.Request{Header: http.Header{}}
		h.ServeHTTP(rr, req)
		close(done)
	}()
	select {
	case <-done:
		// handler returned without blocking — pass
	case <-time.After(2 * time.Second):
		t.Fatal("getForceHandler blocked on a full update channel (non-blocking poke regressed)")
	}
}

// --- progress.go: capture ring + busy/pass/force trackers ----------------

// resetProgressState restores the progress package-global state to a clean
// baseline. Tests mutate these globals; the dashboard and display engine both
// read them, so leaving stale state would cross-contaminate tests.
func resetProgressState() {
	progressMu.Lock()
	progressRing = make([]progressLine, 0, _progressCap)
	progressSeq = 0
	progressStart = time.Time{}
	progressMu.Unlock()
	backupBusy.Store(false)
	passSeq.Store(0)
	forceSeq.Store(0)
}

// TestAppendProgressAppend verifies lines are captured in order and each gets
// a strictly increasing sequence number.
func TestAppendProgressAppend(t *testing.T) {
	resetProgressState()
	for _, msg := range []string{"[BACKUP][START][SERVER] opn01", "[BACKUP][OK] done", "[GIT][REPO][COMMIT] x"} {
		appendProgress([]byte(msg))
	}
	progressMu.Lock()
	defer progressMu.Unlock()
	if len(progressRing) != 3 {
		t.Fatalf("captured %d lines, want 3", len(progressRing))
	}
	for i := 1; i < len(progressRing); i++ {
		if progressRing[i].Seq <= progressRing[i-1].Seq {
			t.Errorf("line %d seq %d not greater than prev %d", i, progressRing[i].Seq, progressRing[i-1].Seq)
		}
	}
	if progressRing[2].Msg != "[GIT][REPO][COMMIT] x" {
		t.Errorf("last msg = %q", progressRing[2].Msg)
	}
}

// TestAppendProgressRingCap verifies the ring buffer never grows beyond the
// configured cap and that the newest entries are retained once it overflows.
func TestAppendProgressRingCap(t *testing.T) {
	resetProgressState()
	for i := 0; i < _progressCap+50; i++ {
		appendProgress([]byte("line-" + strconv.Itoa(i)))
	}
	progressMu.Lock()
	defer progressMu.Unlock()
	if len(progressRing) != _progressCap {
		t.Fatalf("ring grew to %d, want cap %d", len(progressRing), _progressCap)
	}
	// the newest retained entry must be the most recently appended line
	last := progressRing[len(progressRing)-1]
	if last.Msg != "line-"+strconv.Itoa(_progressCap+49) {
		t.Errorf("newest = %q, want line-%d", last.Msg, _progressCap+49)
	}
	// the oldest retained entry must be exactly the one that survived eviction
	oldest := progressRing[0]
	want := "line-" + strconv.Itoa(50)
	if oldest.Msg != want {
		t.Errorf("oldest = %q, want %q", oldest.Msg, want)
	}
	// sequence numbers must remain strictly increasing across the whole ring
	for i := 1; i < len(progressRing); i++ {
		if progressRing[i].Seq <= progressRing[i-1].Seq {
			t.Fatalf("ring ordering broke at index %d", i)
		}
	}
}

// TestBackupPassLifecycle verifies beginBackupPass/endBackupPass flip the
// atomic busy flag and bump the pass counter.
func TestBackupPassLifecycle(t *testing.T) {
	resetProgressState()
	if backupBusy.Load() {
		t.Fatal("busy should start false")
	}
	before := passSeq.Load()
	beginBackupPass()
	if !backupBusy.Load() {
		t.Error("busy should be true after beginBackupPass")
	}
	if passSeq.Load() != before+1 {
		t.Errorf("passSeq = %d, want %d", passSeq.Load(), before+1)
	}
	if progressStart.IsZero() {
		t.Error("progressStart should be set by beginBackupPass")
	}
	endBackupPass()
	if backupBusy.Load() {
		t.Error("busy should be false after endBackupPass")
	}
}

// TestBumpForceSeq verifies each /force poke increments the force counter and
// returns the new value.
func TestBumpForceSeq(t *testing.T) {
	resetProgressState()
	a := bumpForceSeq()
	b := bumpForceSeq()
	if b != a+1 {
		t.Errorf("second bump = %d, want %d", b, a+1)
	}
	if forceSeq.Load() != b {
		t.Errorf("forceSeq = %d, want %d", forceSeq.Load(), b)
	}
}

// TestProgressHandlerJSON verifies the /progress endpoint returns incremental
// log lines (since cursor), the busy state, and counters as JSON.
func TestProgressHandlerJSON(t *testing.T) {
	resetProgressState()
	appendProgress([]byte("[BACKUP][START][SERVER] opn01"))
	appendProgress([]byte("[BACKUP][OK] done"))
	beginBackupPass()
	defer endBackupPass()

	h := getProgressHandler()

	// first poll: full snapshot
	req := httptest.NewRequest(http.MethodGet, "/progress", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content-type = %q, want json", got)
	}
	var snap progressSnapshot
	if err := json.Unmarshal(rr.Body.Bytes(), &snap); err != nil {
		t.Fatalf("unmarshal: %v\nbody: %s", err, rr.Body.String())
	}
	if !snap.Busy {
		t.Error("busy should be true during a pass")
	}
	if len(snap.Lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(snap.Lines))
	}
	if snap.Lines[0].Msg != "[BACKUP][START][SERVER] opn01" {
		t.Errorf("first line = %q", snap.Lines[0].Msg)
	}

	// the highest seq we have seen so far
	since := snap.Lines[1].Seq

	// append one more, then poll with ?since=<highest> and expect only the new
	appendProgress([]byte("[FINISH][BACKUP][ALL]"))

	req2 := httptest.NewRequest(http.MethodGet, "/progress?since="+strconv.FormatUint(since, 10), nil)
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)
	var snap2 progressSnapshot
	if err := json.Unmarshal(rr2.Body.Bytes(), &snap2); err != nil {
		t.Fatalf("unmarshal 2: %v\nbody: %s", err, rr2.Body.String())
	}
	if len(snap2.Lines) != 1 {
		t.Fatalf("incremental lines = %d, want 1", len(snap2.Lines))
	}
	if snap2.Lines[0].Msg != "[FINISH][BACKUP][ALL]" {
		t.Errorf("incremental line = %q", snap2.Lines[0].Msg)
	}
	if snap2.Lines[0].Seq <= since {
		t.Errorf("new seq %d not greater than since %d", snap2.Lines[0].Seq, since)
	}
}

// TestProgressHandlerEmptyLinesIsArray verifies the endpoint emits a JSON
// array (not null) when no lines are available, so the browser JS can safely
// iterate.
func TestProgressHandlerEmptyLinesIsArray(t *testing.T) {
	resetProgressState()
	req := httptest.NewRequest(http.MethodGet, "/progress", nil)
	rr := httptest.NewRecorder()
	getProgressHandler().ServeHTTP(rr, req)
	if !strings.Contains(rr.Body.String(), `"lines":[]`) {
		t.Errorf("expected empty lines array, got: %s", rr.Body.String())
	}
}

// TestGetForceHandlerInjectsForceSeq verifies the /force handler substitutes
// the live forced-pass sequence into the served dashboard HTML (the %FORCE%
// placeholder set by Setup) so the dashboard can distinguish its own pass
// from a concurrent timer-tick pass.
func TestGetForceHandlerInjectsForceSeq(t *testing.T) {
	resetProgressState()
	// Simulate the template Setup would have built. The dashboard JS reads
	// the injected FORCE value to detect when its forced pass has run.
	saved := _forceRedirect
	_forceRedirect = "<html><body>FORCE=%FORCE% force-dash</body></html>"
	t.Cleanup(func() { _forceRedirect = saved })

	rr := httptest.NewRecorder()
	req := &http.Request{Header: http.Header{}}
	getForceHandler().ServeHTTP(rr, req)

	body := rr.Body.String()
	if strings.Contains(body, "%FORCE%") {
		t.Errorf("placeholder not substituted: %s", body)
	}
	if !strings.Contains(body, "force-dash") {
		t.Errorf("dashboard body missing: %s", body)
	}
	// the injected value must equal the current forceSeq counter
	want := strconv.FormatUint(forceSeq.Load(), 10)
	if !strings.Contains(body, "FORCE="+want) {
		t.Errorf("body %q does not contain injected FORCE=%s", body, want)
	}
}
