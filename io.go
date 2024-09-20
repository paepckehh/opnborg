package opnborg

import (
	"encoding/xml"
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
var displayChan, display = make(chan []byte, 20), sync.WaitGroup{}

// startLog is a non-blocking, conditional, concurrent-save background output handler
func startLog(config *OPNCall) {
	if !config.Log {
		go func() {
			for msg := range displayChan {
				outSlice(msg, config)
			}
			display.Done()
		}()
		return

	}
	display.Done()
}

//
// Little Helper
//

func isValidXML(s string) bool {
	return xml.Unmarshal([]byte(s), new(interface{})) == nil
}

func padMonth(in string) string {
	if len(in) == 1 {
		return "0" + in
	}
	return in
}
