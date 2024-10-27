package main

import (
	"fmt"
	"os"
	"time"

	"paepcke.de/opnborg"
)

const (
	_app = "[OPNBORG-CLI]"
)

func main() {

	// Startup
	t0 := time.Now()
	fmt.Println(_app + "[STARTUP][API-VERSION:" + opnborg.SemVer + "]")

	// Configure
	config, err := opnborg.Setup()
	if err != nil {
		fmt.Printf(_app+"[ERROR][EXIT] %s\n", err)
		os.Exit(1)
	}

	// Perform Backup of all Appliances xml configuration
	err = opnborg.Start(config)
	if err != nil {
		fmt.Printf(_app+"[ERROR][EXIT] %s\n", err)
		os.Exit(1)
	}

	// Finish
	fmt.Println(_app + "[END][RUNTIME]:" + time.Since(t0).String())
}
