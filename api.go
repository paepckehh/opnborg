package opnborg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// global const
const _version = "v0.1.1"

// global var
var (
	tg                                                                                                []OPNGroup
	sleep, borg, pkgmaster, wazuhWebUI, prometheusWebUI, grafanaWebUI, grafanaFreeBSD, grafanaHAProxy string
)

// OPNGroup Type
type OPNGroup struct {
	Name   string   // group name
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

// Setup reads OPNBorgs configuration via env, sanitizes, sets sane defaults
func Setup() (*OPNCall, error) {

	// check if setup requirements are meet
	if err := checkSetRequired(); err != nil {
		return nil, err
	}

	// setup from env
	config := &OPNCall{
		Targets:   os.Getenv("OPN_TARGETS"),
		Key:       os.Getenv("OPN_APIKEY"),
		Secret:    os.Getenv("OPN_APISECRET"),
		TLSKeyPin: os.Getenv("OPN_TLSKEYPIN"),
		Path:      os.Getenv("OPN_PATH"),
		Email:     os.Getenv("OPN_EMAIL"),
	}

	// setup app
	if config.AppName == "" {
		config.AppName = "[OPNBORG-API]"
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
	// configure remote syslog server
	config.RSysLog.Enable = false
	if config.Daemon {
		if _, ok := os.LookupEnv("OPN_RSYSLOG_ENABLE"); ok {
			if _, ok := os.LookupEnv("OPN_RSYSLOG_SERVER"); ok {
				config.RSysLog.Enable = true
				config.RSysLog.Server = os.Getenv("OPN_RSYSLOG_SERVER")
				if len(strings.Split(config.RSysLog.Server, ":")) < 1 {
					return nil, errors.New(fmt.Sprintf("env var 'OPN_RSYSLOG_SRV' format error, example \"192.168.0.100:5140\""))
				}
			}
		}
	}
	// configure httpd
	config.Httpd.Enable = true
	if config.Daemon {
		if _, ok := os.LookupEnv("OPN_HTTPD_ENABLE"); ok {
			if _, ok := os.LookupEnv("OPN_HTTPD_SERVER"); ok {
				config.Httpd.Enable = true
				config.Httpd.Server = os.Getenv("OPN_HTTPD_SERVER")
				if config.Httpd.Server == "" {
					config.Httpd.Server = "127.0.0.1:6464"
				}
				if len(strings.Split(config.Httpd.Server, ":")) < 1 {
					return nil, errors.New(fmt.Sprintf("env var 'OPN_HTTPD_SRV' format error, example \"127.0.0.1:6464\""))
				}
				config.Httpd.CAcert = os.Getenv("OPN_HTTPD_CACERT")
				config.Httpd.CAkey = os.Getenv("OPN_HTTPD_CAKEY")
				config.Httpd.CAClient = os.Getenv("OPN_HTTPD_CACLIENT")
			}
		}
	}
	// config Master
	config.Sync.Enable = false
	config.Sync.validConf = false
	config.Sync.PKG.Enable = false
	if _, ok := os.LookupEnv("OPN_MASTER"); ok {
		config.Sync.Enable = true
		config.Sync.Master = os.Getenv("OPN_MASTER")
		if _, ok := os.LookupEnv("OPN_SYNC_PKG"); ok {
			config.Sync.PKG.Enable = true
			pkgmaster = "https://" + config.Sync.Master + _plug
		}
	}
	// prometheus
	if _, ok := os.LookupEnv("OPN_PROMETHEUS_WEBUI"); ok {
		config.Prometheus.Enable = true
		config.Prometheus.WebUI = os.Getenv("OPN_PROMETHEUS_WEBUI")
		config.Prometheus.WebUI = config.Prometheus.WebUI
		prometheusWebUI = config.Prometheus.WebUI
	}
	// wazuh
	if _, ok := os.LookupEnv("OPN_WAZUH_WEBUI"); ok {
		config.Wazuh.Enable = true
		config.Wazuh.WebUI = os.Getenv("OPN_WAZUH_WEBUI")
		config.Wazuh.WebUI = config.Wazuh.WebUI
		wazuhWebUI = config.Wazuh.WebUI
	}
	// grafana
	if _, ok := os.LookupEnv("OPN_GRAFANA_WEBUI"); ok {
		config.Grafana.Enable = true
		config.Grafana.WebUI = os.Getenv("OPN_GRAFANA_WEBUI")
		config.Grafana.WebUI = config.Grafana.WebUI
		grafanaWebUI = config.Grafana.WebUI
		if _, ok := os.LookupEnv("OPN_GRAFANA_DASHBOARD_FREEBSD"); ok {
			config.Grafana.FreeBSD = os.Getenv("OPN_GRAFANA_DASHBOARD_FREEBSD")
			grafanaFreeBSD = config.Grafana.FreeBSD
		}
		if _, ok := os.LookupEnv("OPN_GRAFANA_DASHBOARD_HAPROXY"); ok {
			config.Grafana.HAProxy = os.Getenv("OPN_GRAFANA_DASHBOARD_HAPROXY")
			grafanaHAProxy = config.Grafana.HAProxy
		}
	}
	// configure eMail default
	if config.Email == "" {
		config.Email = "git@opnborg"
	}
	// configure sleep for daemon mode
	sleep = "0"
	if config.Daemon {
		config.Sleep = 3600
		if sleep, ok := os.LookupEnv("OPN_SLEEP"); ok {
			var err error
			config.Sleep, err = strconv.ParseInt(sleep, 10, 64)
			if err != nil {
				return nil, errors.New(fmt.Sprintf("env var 'OPN_SLEEP' must contain a number in seconds without prefix or suffix"))
			}
		}
		if config.Sleep < 10 {
			config.Sleep = 10
		}
		sleep = strconv.FormatInt(config.Sleep, 10)
	}
	config.extGIT = true
	return config, nil
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
	displayChan <- []byte("[STARTING][" + _app + "][" + _version + "]" + suffix)

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
