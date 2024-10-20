package opnborg

import (
	"encoding/xml"
	"fmt"
	"os"
	"sort"
	"strings"
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
func checkSetRequired() error {

	if _, ok := os.LookupEnv("OPN_APIKEY"); !ok {
		return fmt.Errorf("set env variable 'OPN_APIKEY' to your opnsense api key")
	}

	if _, ok := os.LookupEnv("OPN_APISECRET"); !ok {
		return fmt.Errorf("set env variable 'OPN_APISECRET' to your opnsense api key secret")
	}
	if _, ok := os.LookupEnv("OPN_TARGETS"); !ok {
		member := ""
		env := os.Environ()
		if len(env) > 1 {
			sort.Strings(env)
			for _, value := range env {
				if len(value) > 15 {
					if value[0:12] == "OPN_TARGETS_" {
						grp := strings.Split(value, "=")
						if len(member) > 0 {
							member = member + ","
						}
						member = member + grp[1]
						tg = append(tg, OPNGroup{Name: grp[0][12:], Member: strings.Split(grp[1], ",")})
					}
				}
			}
			if len(member) > 0 {
				os.Setenv("OPN_TARGETS", member)
				return nil
			}
		}
		return fmt.Errorf("add at least one target server to env var 'OPN_TARGETS' or 'OPN_TARGETS_* '(multi valued, comma seperated)")
	}
	tg = append(tg, OPNGroup{Name: "Hive", Member: strings.Split(os.Getenv("OPN_TARGETS"), ",")})
	return nil
}
