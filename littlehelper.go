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

// outSlice write messages to stdout
func outSlice(msg []byte, config *OPNCall) {
	os.Stdout.Write([]byte(config.AppName))
	os.Stdout.Write(msg)
	os.Stdout.Write([]byte("\n"))
}

// displayChan channel for the display engine
var displayChan, display, wg = make(chan []byte, 20), sync.WaitGroup{}, sync.WaitGroup{}

// startLog is a non-blocking, conditional, concurrent-save background output handler
func startLog(config *OPNCall) {
	go func() {
		for msg := range displayChan {
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
	return xml.Unmarshal([]byte(s), new(interface{})) == nil
}

// padMonth
func padMonth(in string) string {
	if len(in) == 1 {
		return "0" + in
	}
	return in
}
