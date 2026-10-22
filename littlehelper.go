package opnborg

import (
	"encoding/xml"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
)

//
// Display IO
//

// ANSI SGR codes used by the display engine. Color is applied only when
// stdout is a terminal, so piped output, container logs, the WebUI progress
// ring buffer and test output stay free of escape sequences.
const (
	_colorReset  = "\033[0m"
	_colorRed    = "\033[31m"
	_colorGreen  = "\033[32m"
	_colorYellow = "\033[33m"
	_colorCyan   = "\033[36m"
)

// colorOn is resolved once at startup: interactive stdout and no explicit
// opt-out via NO_COLOR or OPN_NOCOLOR (which is presence-based, like every
// other OPN_* boolean).
var colorOn = colorEnabled()

// colorEnabled reports whether the display engine should colorize stdout.
func colorEnabled() bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	if isEnv("OPN_NOCOLOR") {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// containsAny reports whether line contains at least one of the needles.
func containsAny(line string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(line, n) {
			return true
		}
	}
	return false
}

// colorizeLine classifies a display line by its status tags and wraps it in
// the matching ANSI color. Unknown lines pass through unchanged. The classifier
// is deliberately tag-based so it never alters the payload the progress ring
// buffer and syslog consumers rely on.
func colorizeLine(msg []byte, enabled bool) string {
	line := string(msg)
	if !enabled || line == "" {
		return line
	}
	code := ""
	switch {
	case containsAny(line, "[ERROR]", "[FAIL", "[EXIT]", "[DENIED]"):
		code = _colorRed
	case containsAny(line, "[WARN]"):
		code = _colorYellow
	case containsAny(line, "[OK]", "[SUCCESS", "[FINISH]"):
		code = _colorGreen
	case containsAny(line, "[STARTING]", "[START]", "[INFO]"):
		code = _colorCyan
	}
	if code == "" {
		return line
	}
	return code + line + _colorReset
}

// outSlice write messages to stdout in a single write so the display
// engine never interleaves partial lines under concurrent producers.
func outSlice(msg []byte, config *OPNCall) {
	line := colorizeLine(msg, colorOn)
	out := make([]byte, 0, len(config.AppName)+len(line)+1)
	out = append(out, config.AppName...)
	out = append(out, line...)
	out = append(out, '\n')
	_, _ = os.Stdout.Write(out)
}

// displayChan channel for the display engine
var displayChan, display, wg = make(chan []byte, 20), sync.WaitGroup{}, sync.WaitGroup{}

// _cfg is the package-level handle on the live OPNCall config, captured when
// the httpd arms so the WebUI render path (which has no config parameter) can
// reach the storage path and git settings for the dashboard panels.
var _cfg *OPNCall

// startLog is a non-blocking, conditional, concurrent-save background output handler
func startLog(config *OPNCall) {
	go func() {
		for msg := range displayChan {
			appendProgress(msg)
			outSlice(msg, config)
		}
		display.Done()
	}()
}

//
// Little Helper
//

// checkURL parses the URL carried by the given env var. A set-but-empty (or
// whitespace-only) value is treated as unset: url.Parse("") happily returns
// a non-nil empty URL, which downstream nil-checks treat as a configured
// feature and every request built from it fails.
func checkURL(env string) (*url.URL, error) {
	val, ok := os.LookupEnv(env)
	if !ok || strings.TrimSpace(val) == "" {
		return nil, nil
	}
	out, err := url.Parse(val)
	if err != nil {
		return nil, errors.New("[SETUP][" + env + "][INVALID-URL] " + err.Error())
	}
	return out, nil
}

// checkPreURL check prefixed url
func checkPreURL(base *url.URL, prefix, env string) (*url.URL, error) {
	out, err := url.Parse(base.String() + prefix + os.Getenv(env))
	if err != nil {
		return nil, errors.New("[SETUP][" + env + "][INVALID-URL] " + err.Error())
	}
	return out, nil
}

// isEnv
func isEnv(check string) bool {
	if content, ok := os.LookupEnv(check); ok {
		if content != "" {
			return true
		}
	}
	return false
}

// isValidXML
func isValidXML(s string) bool {
	return xml.Unmarshal([]byte(s), new(any)) == nil
}

// padMonth
func padMonth(in string) string {
	if len(in) == 1 {
		return "0" + in
	}
	return in
}
