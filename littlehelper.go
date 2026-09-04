package opnborg

import (
	"encoding/xml"
	"errors"
	"net/url"
	"os"
	"sync"
)

//
// Display IO
//

// outSlice write messages to stdout in a single write so the display
// engine never interleaves partial lines under concurrent producers.
func outSlice(msg []byte, config *OPNCall) {
	out := make([]byte, 0, len(config.AppName)+len(msg)+1)
	out = append(out, config.AppName...)
	out = append(out, msg...)
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

// checkURL
func checkURL(env string) (*url.URL, error) {
	if _, ok := os.LookupEnv(env); ok {
		out, err := url.Parse(os.Getenv(env))
		if err != nil {
			return nil, errors.New("[SETUP][" + env + "][INVALID-URL] " + err.Error())
		}
		return out, nil
	}
	return nil, nil
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
