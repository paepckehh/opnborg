package main

import (
	"fmt"
	"os"

	"paepcke.de/opnborg"
)

const _app = "[OPNBORG-CLI]"

func main() {

	fmt.Println(_app + "[STARTUP][V0.0.1]")

	// Read Application Env
	config, err := opnborg.ReadConfig()
	if err != nil {
		fmt.Printf(_app+"[EXIT]%s\n", err)
		os.Exit(1)
	}

	// Perform Backup of all Appliances xml configuration
	err = opnborg.Backup(config)
	if err != nil {
		fmt.Printf(_app+"[EXIT]%s\n", err)
		os.Exit(1)
	}
	fmt.Println(_app + "[END]")
}
