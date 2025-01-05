package opnborg

import (
	"net/url"
	"sync/atomic"
)

// global exported consts
const SemVer = "v0.1.54"

// global var
var (
	tg                                                         []OPNGroup
	unifiBackupEnable, unifiExportEnable                       atomic.Bool
	unifiBackupNow, unifiExportNow                             atomic.Bool
	sleep, borg, pkgmaster, pkghost                            string
	wazuhWebUI, unifiWebUI, prometheusWebUI                    *url.URL
	grafanaWebUI, grafanaFreeBSD, grafanaUnifi, grafanaHAProxy *url.URL
)

// OPNGroup Type
type OPNGroup struct {
	Name   string   // group name
	OPN    bool     // is OPNsense Appliance
	Unifi  bool     // is Unifi Controller
	Img    bool     // group image available
	ImgURL string   // group image url
	Member []string // group member
}

// OPNCall
type OPNCall struct {
	Enable    bool        // enable OPNsense Backup mode
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
		WebUI   *url.URL
		Version string
		Backup  struct {
			Enable bool
			User   string
			Secret string
		}
		Export struct {
			Enable bool
			URI    *url.URL
		}
	}
	Wazuh struct {
		WebUI *url.URL
	}
	Prometheus struct {
		WebUI *url.URL
	}
	Grafana struct {
		WebUI   *url.URL
		FreeBSD *url.URL
		HAProxy *url.URL
		Unifi   *url.URL
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

// Start Application Server
func Start(config *OPNCall) error {
	return srv(config)
}
