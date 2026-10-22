package main

import (
	"log"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"paepcke.de/opnborg"
)

const (
	_app = "[OPNBORG-CLI]"
)

// revision returns the VCS commit the binary was built from, shortened to the
// usual 7 characters, with a "-dirty" marker for uncommitted builds. It falls
// back to "unknown" for builds without embedded VCS metadata (module builds,
// stripped/repackaged binaries), so the startup banner always has a value.
func revision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	var rev string
	var dirty string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "-dirty"
			}
		}
	}
	if rev == "" {
		return "unknown"
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	return rev + dirty
}

func main() {

	t0 := time.Now()

	// Check commandline
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-v", "--version":
			log.Println(_app + "[VERSION:" + opnborg.SemVer + "][COMMIT:" + revision() + "]")
			os.Exit(0)
		case "-h", "--help":
			log.Println(_app + "[HELP] The only supported commandline options are [-v|--version] [-h|--help]")
			log.Println(_app + "[HELP] Configure OPNBorg via ENV only.")
			log.Println(_app + "[HELP] Visit [paepcke.de/opnborg|github.com/paepckehh/opnborg] for details.")
			os.Exit(0)
		default:
			log.Fatalf(_app+"[ERROR][EXIT][COMMANDLINE-OPTIONS-NOT-SUPPORTED]: %s\n", strings.Join(os.Args[1:], " "))
		}
	}

	// Startup: version (must equal the newest git tag) plus the exact commit
	// the binary was built from, so an operator can verify the deployed artifact.
	log.Println(_app + "[STARTUP][VERSION:" + opnborg.SemVer + "][COMMIT:" + revision() + "]")

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
