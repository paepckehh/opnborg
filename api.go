package opnborg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// OPNCall
type OPNCall struct {
	Targets   string // list of OPNSense Appliances, csv comma seperated
	Key       string // OPNSense Backup User API Key (required)
	Secret    string // OPNSense Backup User API Secret (required)
	Path      string // OPNSense Backup Files Target Path, default:'.'
	TLSKeyPin string // TLS Connection Server Certificate KeyPIN
	AppName   string // Display and SysLog Application Name
	Sleep     int64  // number of seconds to sleep between polls
	Daemon    bool   // daemonize (run in background), default: false
	SSL       bool   // enforce verify SSL trustchain against system SSL Trust store (use TLSKeyPIN), default: false
	Git       bool   // create and commit all xml files & changes to local .git repo, default: true
	Log       bool   // if true, write to syslog (daemon mode) instead to stdout, default: false
	Debug     bool   // defaults to false
}

// Setup reads OPNBorgs configuration via env, sanitizes, sets sane defaults
func Setup() (*OPNCall, error) {

	// check if setup requirements are meet
	if err := checkRequired(); err != nil {
		return nil, err
	}

	// setup from env
	config := &OPNCall{
		Targets:   os.Getenv("OPN_TARGETS"),
		Key:       os.Getenv("OPN_APIKEY"),
		Secret:    os.Getenv("OPN_APISECRET"),
		Path:      os.Getenv("OPN_PATH"),
		TLSKeyPin: os.Getenv("OPN_TLSKEYPIN"),
	}

	// sanitize input
	if config.Path == "" {
		config.Path = filepath.Dir("./")
	}

	// validate bools, set defaults
	if _, ok := os.LookupEnv("OPN_DAEMON"); ok {
		config.Daemon = true
	}
	if _, ok := os.LookupEnv("OPN_NOSSL"); ok {
		config.SSL = true
	}
	if _, ok := os.LookupEnv("OPN_DEBUG"); ok {
		config.Debug = true
	}
	config.Git = true
	if _, ok := os.LookupEnv("OPN_NOGIT"); ok {
		config.Git = false
	}
	if sleep, ok := os.LookupEnv("OPN_SLEEP"); ok {
		var err error
		config.Sleep, err = strconv.ParseInt(sleep, 10, 64)
		if err != nil {
			return nil, errors.New(fmt.Sprintf("if env var 'OPN_SLEEP' is set, it must contain a number in seconds without prefix or suffix"))
		}
	}
	if config.Sleep < 5 {
		config.Sleep = 5
	}
	return config, nil
}

// Backup performs a Backup operation
func Backup(config *OPNCall) error {

	// setup
	var wg, display sync.WaitGroup
	if config.AppName == "" {
		config.AppName = "[OPNBORG-API]"
	}

	// spinup Log/Display Engine
	display.Add(1)
	go startLog(config)

	servers := strings.Split(config.Targets, ",")
	for {

		// spinup individual worker for every server
		if config.Debug {
			displayChan <- []byte("[STARTING][BACKUP]")
		}
		for _, server := range servers {
			wg.Add(1)
			go backupSrv(server, config, &wg)
		}

		// wait till all worker done
		wg.Wait()
		if config.Debug {
			displayChan <- []byte("[FINISH][BACKUP][ALL]")
		}
		if !config.Daemon {
			close(displayChan)
			display.Wait()
			return nil
		}
		time.Sleep(time.Duration(config.Sleep) * time.Second)
	}
}
