package opnborg

import (
	"encoding/xml"
	"errors"
	"fmt"
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
	return

}

//
// Little Helper
//

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

// checkRequired env input
func checkRequired() error {
	if _, ok := os.LookupEnv("OPN_TARGETS"); !ok {
		return errors.New(fmt.Sprintf("Add at least one target server to env var 'OPN_TARGETS' (multi valued, comma seperated)"))
	}

	if _, ok := os.LookupEnv("OPN_APIKEY"); !ok {
		return errors.New(fmt.Sprintf("Set env var 'OPN_APIKEY' to your opnsense api key"))
	}

	if _, ok := os.LookupEnv("OPN_APISECRET"); !ok {
		return errors.New(fmt.Sprintf("Set env var 'OPN_APISECRET' to your opnsense api key secret"))
	}
	return nil
}
