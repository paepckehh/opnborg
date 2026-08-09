package opnborg

import (
	"net/url"
	"sync/atomic"
	"time"
)

// global exported consts
const SemVer = "v0.1.161"

// global var
var (
	tg                                                         []OPNGroup
	unifiBackupEnable, unifiExportEnable, unifiWatchEnable     atomic.Bool
	unifiBackupNow, unifiExportNow, unifiWatchNow              atomic.Bool
	sleep, pkgmaster, pkghost                                  string
	wazuhWebUI, unifiWebUI, prometheusWebUI                    *url.URL
	grafanaWebUI, grafanaFreeBSD, grafanaUnifi, grafanaHAProxy *url.URL
	unifiWatchPath                                             string
)

// OPNGroup Type
type OPNGroup struct {
	Name   string   // group name
	OPN    bool     // is OPNsense Appliance
	Unifi  bool     // is Unifi Controller
	Desc   string   // group description (text, from env, shown when no ImgURL)
	ImgURL string   // group image url (from env, replaces text headline when set)
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
	Git struct {
		Enable   bool   // manage the backup storage folder as a local git repo with auto commit (default: false)
		Upstream string // upstream SSH git URL to sync with (e.g. git@github.com:user/repo.git); empty disables push
		SSHKey   string // path to the PEM-encoded SSH private key used for upstream auth (required when Upstream is set)
		// SSHHostKey is the optional SHA-256 fingerprint of the upstream SSH
		// host public key (format "SHA256:base64...", matching the output of
		// `ssh-keyscan -t rsa,ed25519 <host>` / ssh's "Server host key"
		// banner). When set, the upstream connection refuses any host whose
		// presented fingerprint does not match. When unset, host key
		// verification is skipped entirely (insecure): opnborg never writes
		// or reads a known_hosts file, so in unattended container/CI
		// deployments where $HOME is undefined this avoids the go-git
		// "cannot create known hosts callback" fatal exit. Pin a fingerprint
		// whenever the upstream is reachable over an untrusted network.
		SSHHostKey string
	}
	Ollama struct {
		Enable bool   // enable Ollama-assisted git commit message generation (requires URL + Model)
		URL    string // Ollama REST API base URL (e.g. http://localhost:11434); appended with /api/generate
		Model  string // Ollama model name used to summarise each backup diff (e.g. llama3)
	}
	Unifi struct {
		WebUI   *url.URL
		Tag     string
		Version string
		Backup  struct {
			Enable bool
			User   string
			Secret string
		}
		Export struct {
			Enable bool
			Format string
			URI    *url.URL
		}
		Watch struct {
			Enable bool      // enable Unifi autoBackup folder watch & sync
			Path   string    // absolute path to the Unifi autoBackup source folder
			Meta   string    // absolute path to the autobackup_meta.json marker file
			LastTS time.Time // mtime of the marker file at the last successful sync
			// runtime sync stats (guarded by unifiWatchMutex, see status.go)
			SetupErr     string    // setup-time error reason when the watcher could not be armed (empty when armed OK)
			LastSyncErr  string    // last runtime sync failure reason (cleared on the next successful sync)
			LastSyncTS   time.Time // wall-clock of the last sync pass
			LastFile     string    // newest .unf file synced on the last pass
			SyncedFiles  int       // number of new .unf files checked into the store on the last pass
			SkippedFiles int       // number of .unf files that were unchanged (sha256 matched current.unf)
			SourceFiles  int       // total number of .unf files observed in the source folder on the last pass
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
