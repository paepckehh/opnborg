package opnborg

import (
	"os"
	"sync"
)

//
// Display IO
//

// outSlice ...
func outSlice(msg []byte, config *OPNCall) {
	os.Stdout.Write([]byte(config.AppName))
	os.Stdout.Write(msg)
	os.Stdout.Write([]byte("\n"))
}

// global display engine
var displayChan, display = make(chan []byte, 20), sync.WaitGroup{}

// startLog, a non-blocking, conditional, concurrent-save background output handler
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
