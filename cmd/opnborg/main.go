package main

import (
	"log"
	"time"

	"paepcke.de/opnborg"
)

const (
	_app = "[OPNBORG-CLI]"
)

func main() {

	// Startup
	t0 := time.Now()
	log.Println(_app + "[STARTUP][API-VERSION:" + opnborg.SemVer + "]")

	// Configure
	config, err := opnborg.Setup()
	if err != nil {
		log.Fatalf(_app+"[ERROR][EXIT] %s\n", err)
	}

	// Perform Backup of all Appliances xml configuration
	err = opnborg.Start(config)
	if err != nil {
		log.Fatalf(_app+"[ERROR][EXIT] %s\n", err)
	}

	// Finish
	log.Println(_app + "[END][RUNTIME]:" + time.Since(t0).String())
}
