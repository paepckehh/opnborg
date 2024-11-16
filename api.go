package opnborg

import (
	"sync"
	"sync/atomic"
)

// global exported consts
const SemVer = "v0.1.35"

// global var
var (
	tg                                                            []OPNGroup
	sleep, borg, pkgmaster                                        string
	wazuhWebUI, unifiWebUI, unifiDash, prometheusWebUI            string
	grafanaWebUI, grafanaFreeBSD, grafanaUnpoller, grafanaHAProxy string
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
	GitPush   bool        // push .git repo to configured upstream, default: false
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
	Unifi struct {
		Enable    bool
		WebUI     string
		Dashboard string
		Backup    struct {
			Enable bool
			User   string
			Secret string
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
var update = make(chan bool, 1)

// Start Application Server
func Start(config *OPNCall) error {
	return srv(config)
}
