package opnborg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// OPNCall
type OPNCall struct {
	Targets    string      // list of OPNSense Appliances, csv comma seperated
	Key        string      // OPNSense Backup User API Key (required)
	Secret     string      // OPNSense Backup User API Secret (required)
	Path       string      // OPNSense Backup Files Target Path, default:'.'
	TLSKeyPin  string      // TLS Connection Server Certificate KeyPIN
	AppName    string      // Display and SysLog Application Name
	Email      string      // Git Commiter eMail Address (default: git@opnborg)
	CAcert     string      // httpd server certificate (pem encoded x.509 certificate chain)
	CAkey      string      // httpd server key (pem encoded key)
	CAclient   string      // httpd client CA (will enforce mTLS only mode)
	ListenAddr string      // HTTPD Listen IP and Port
	Sleep      int64       // number of seconds to sleep between polls
	Daemon     bool        // daemonize (run in background), default: false
	Debug      bool        // verbose debug logs, defaults to false
	Git        bool        // create and commit all xml files & changes to local .git repo, default: true
	RSysLog    bool        // enable the internal rsyslog server instance & auto configure all targets (req. daemon mode)
	extGIT     bool        // when available, use external git for verification
	dirty      atomic.Bool // git global (atomic) worktree state
	Sync       struct {
		Enable bool   // enable Master Server
		Master string // Master Server Name
		USR    struct {
			Enable bool // enable user account sync
		}
		PKG struct {
			Enable   bool     // enable packages sync
			Packages []string // list of Packages to sync
		}
	}
}

// Setup reads OPNBorgs configuration via env, sanitizes, sets sane defaults
func Setup() (*OPNCall, error) {

	// check if setup requirements are meet
	if err := checkRequired(); err != nil {
		return nil, err
	}

	// setup from env
	config := &OPNCall{
		Targets:    os.Getenv("OPN_TARGETS"),
		Key:        os.Getenv("OPN_APIKEY"),
		Secret:     os.Getenv("OPN_APISECRET"),
		Path:       os.Getenv("OPN_PATH"),
		Email:      os.Getenv("OPN_EMAIL"),
		TLSKeyPin:  os.Getenv("OPN_TLSKEYPIN"),
		CAcert:     os.Getenv("OPN_CACERT"),
		CAkey:      os.Getenv("OPN_CAKEY"),
		CAclient:   os.Getenv("OPN_CACLIENT"),
		ListenAddr: os.Getenv("OPN_LISTEN"),
	}

	// sanitize input
	if config.Path == "" {
		config.Path = filepath.Dir("./")
	}

	// validate bools, set defaults
	config.Debug = false
	if _, ok := os.LookupEnv("OPN_DEBUG"); ok {
		config.Debug = true
	}
	config.Git = true
	if _, ok := os.LookupEnv("OPN_NOGIT"); ok {
		config.Git = false
	}
	config.Daemon = true
	if _, ok := os.LookupEnv("OPN_NODAEMON"); ok {
		config.Daemon = false
	}
	config.RSysLog = false
	if config.Daemon {
		if _, ok := os.LookupEnv("OPN_RSYSLOG"); ok {
			config.RSysLog = true
		}
	}
	config.Sync.Enable = false
	config.Sync.PKG.Enable = false
	config.Sync.USR.Enable = false
	// config Master
	if _, ok := os.LookupEnv("OPN_MASTER"); ok {
		config.Sync.Enable = true
		config.Sync.Master = os.Getenv("OPN_MASTER")
		if _, ok := os.LookupEnv("OPN_SYNC_PKG"); ok {
			config.Sync.PKG.Enable = true
		}
		if _, ok := os.LookupEnv("OPN_SYNC_USR"); ok {
			config.Sync.USR.Enable = true
		}
	}
	// configure eMail default
	if config.Email == "" {
		config.Email = "git@opnborg"
	}
	// configure httpd defaults
	if config.ListenAddr == "" {
		config.ListenAddr = "0.0.0.0:6464"
	}
	// configure sleep for daemon mode
	if sleep, ok := os.LookupEnv("OPN_SLEEP"); ok {
		var err error
		config.Sleep, err = strconv.ParseInt(sleep, 10, 64)
		if err != nil {
			return nil, errors.New(fmt.Sprintf("when env var 'OPN_SLEEP' is set, it must contain a number in seconds without prefix or suffix"))
		}
		if config.Daemon == false {
			return nil, errors.New(fmt.Sprintf("env var 'OPN_SLEEP' is defined, but OPN_DAEMON Mode is disabled"))
		}
	} else {
		config.Sleep = 3600
	}
	if config.Sleep < 4 {
		config.Sleep = 4
	}
	config.extGIT = true
	return config, nil
}

// Start Application
func Start(config *OPNCall) error {

	// setup
	if config.AppName == "" {
		config.AppName = "[OPNBORG-API]"
	}

	// spin up internal webserver
	go startWeb(config)

	// spin up internal rsyslog server
	go startRSysLog(config)

	// spin up Log/Display Engine
	display.Add(1)
	go startLog(config)

	servers := strings.Split(config.Targets, ",")
	for {
		// init
		var err error

		// fetch target configuration from master server
		if config.Sync.Enable {
			config, err = readMasterConf(config)
			if err != nil {
				displayChan <- []byte("[MASTER][FAIL-TO-READ-CONFIG]" + err.Error())
			}
		}

		// reset global (atomic) git worktree state tracker
		if config.Git {
			config.dirty.Store(false)
		}

		// spinup individual worker for every server
		if config.Debug {
			displayChan <- []byte("[STARTING][BACKUP]")
		}
		for _, server := range servers {
			wg.Add(1)
			go actionSrv(server, config, &wg)
		}

		// wait till all worker done
		wg.Wait()

		// check files into local git repo
		if config.dirty.Load() {
			if config.Git {
				if err := gitCheckIn(config); err != nil {
					displayChan <- []byte("[GIT][REPO][CHECKIN][FAIL]")
					return err
				}
				displayChan <- []byte("[CHANGES-DETECTED][GIT][REPO][CHECKIN][FINISH]")
			}
			displayChan <- []byte("[CHANGES-DETECTED][UPDATES-DONE][FINISH]")
		}

		// finish
		if config.Debug {
			displayChan <- []byte("[FINISH][BACKUP][ALL]")
		}

		// exit if not in daemon mode
		if !config.Daemon {
			close(displayChan)
			display.Wait()
			return nil
		}

		// wait loop
		time.Sleep(time.Duration(config.Sleep) * time.Second)
	}
}
