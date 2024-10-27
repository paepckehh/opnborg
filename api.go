package opnborg

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// global exported consts
const SemVer = "v0.1.21"

// global var
var (
	tg                                                                        []OPNGroup
	sleep, borg, pkgmaster                                                    string
	wazuhWebUI, prometheusWebUI, grafanaWebUI, grafanaFreeBSD, grafanaHAProxy string
)

// OPNGroup Type
type OPNGroup struct {
	Name   string   // group name
	Img    bool     // group image available
	ImgURL string   // group image url
	Member []string // group member
}

// OPNCall
type OPNCall struct {
	Targets   string      // list of OPNSense Appliances, csv comma seperated
	TGroups   []OPNGroup  // list of OPNSense Appliances Target Groups and Member
	Key       string      // OPNSense Backup User API Key (required)
	Secret    string      // OPNSense Backup User API Secret (required)
	Path      string      // OPNSense Backup Files Target Path, default:'.'
	TLSKeyPin string      // TLS Connection Server Certificate KeyPIN
	AppName   string      // Display and SysLog Application Name
	Email     string      // Git Commiter eMail Address (default: git@opnborg)
	Sleep     int64       // number of seconds to sleep between polls
	Daemon    bool        // daemonize (run in background), default: true
	Debug     bool        // verbose debug logs, defaults to false
	Git       bool        // create and commit all xml files & changes to local .git repo, default: true
	extGIT    bool        // when available, use external git for verification
	dirty     atomic.Bool // git global (atomic) worktree state
	Httpd     struct {
		Enable   bool   // enable internal web server
		Server   string // internal httpd server listen ip & port (string, default: 127.0.0.1:6464)
		CAcert   string // httpd server certificate (path to pem encoded x509 file - full certificate chain)
		CAkey    string // httpd server key (path to pem encoded tls server key file)
		CAClient string // httpd client CA (path to pem endcoded x509 file - if set, it will enforce mTLS-only mode)
		Color    struct {
			FG string // color theme background
			BG string // color theme foreground
		}
	}
	Wazuh struct {
		Enable bool
		WebUI  string
	}
	Prometheus struct {
		Enable bool
		WebUI  string
	}
	Grafana struct {
		Enable  bool
		WebUI   string
		FreeBSD string
		HAProxy string
	}
	GrayLog struct {
		Enable bool   // enable use of graylog server
		Server string // graylog server
	}
	RSysLog struct {
		Enable bool   // enable RFC5424 compliant remote syslog store server (default: false)
		Server string // internal syslog listen ip and port [ example: 192.168.0.100:5140 ] (required)
	}
	Sync struct {
		Enable    bool   // enable Master Server
		validConf bool   // internal state (skip if master conf is invalid/unreachable)
		Master    string // Master Server Name
		PKG       struct {
			Enable   bool     // enable packages sync
			Packages []string // list of Packages to sync
		}
	}
}

// global
var hive []string
var hiveMutex sync.Mutex

// Start Application
func Start(config *OPNCall) error {

	// spin up Log/Display Engine
	display.Add(1)
	go startLog(config)

	// spin up internal webserver
	go startWeb(config)

	// spin up internal rsyslog server
	go startRSysLog(config)

	// setup hive
	servers := strings.Split(config.Targets, ",")
	for _, server := range servers {
		status := _na + " <b>Member: </b> " + server + " <b>Version: </b>n/a <b>Last Seen: </b>n/a<br>"
		hive = append(hive, status)
	}

	// startup app version & state, sleep panic gate
	suffix := "[CLI-ONE-TIME-PASS-MODE]"
	if config.Daemon {
		suffix = "[DAEMON-MODE][SLEEP:" + sleep + " SECONDS]"
	}
	displayChan <- []byte("[STARTING][" + _app + "][" + SemVer + "]" + suffix)

	// loop
	for {
		// init
		var err error

		// fetch target configuration from master server
		if config.Sync.Enable {
			config.Sync.validConf = true
			config, err = readMasterConf(config)
			if err != nil {
				config.Sync.validConf = false
				displayChan <- []byte("[ERROR][UNABLE-TO-READ-MASTER-CONFIG]" + err.Error())
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
		for id, server := range servers {
			wg.Add(1)
			go actionSrv(server, config, id, &wg)
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
