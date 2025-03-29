package main

import (
	"log"
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
	log.Println(_app + "[STARTUP][API-VERSION:" + opnborg.SemVer + "]")

	// Check commandline
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-v", "--version":
			os.Exit(0)
		case "-h", "--help":
			log.Println(_app + "[HELP] The only supported commandline options are [-v|--version] [-h|--help]")
			log.Println(_app + "[HELP] Configure OPNBorg via ENV only.")
			log.Println(_app + "[HELP] Visit [paepcke.de/opnborg|github.com/paepckehh/opnborg] for details.")
			os.Exit(0)
		default:
			log.Fatalf(_app+"[ERROR][EXIT][COMMANDLINE-OPTIONS-NOT-SUPPORTED]: %s\n", os.Args[1:])
		}
	}

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
