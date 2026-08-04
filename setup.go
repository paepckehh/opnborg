package opnborg

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/joho/godotenv"
)

// env var prefix constants used by checkSetRequiredOPN to walk OPN_TARGETS_*
// group definitions out of the process environment.
const (
	_opnTargetsPrefix     = "OPN_TARGETS_"
	_opnTargetsDescPrefix = "OPN_TARGETS_DESC_"
	_opnTargetsImgPrefix  = "OPN_TARGETS_IMGURL_"
)

// global
var (
	hive                                   []string
	hiveMutex, unifiMutex, unifiWatchMutex sync.Mutex
	updateOPN                              = make(chan bool, 1)
	updateUnifiBackup                      = make(chan bool, 1)
	updateUnifiExport                      = make(chan bool, 1)
	updateUnifiWatch                       = make(chan bool, 1)
	unifiStatus                            string
	unifiWatchStatus                       string
)

// Setup reads OPNBorgs configuration via env, sanitizes, sets sane defaults
func Setup() (*OPNCall, error) {
	// load .env
	_ = godotenv.Load()

	// var
	var err error

	// setup from env
	config := &OPNCall{
		Key:       os.Getenv("OPN_APIKEY"),
		Secret:    os.Getenv("OPN_APISECRET"),
		TLSKeyPin: os.Getenv("OPN_TLSKEYPIN"),
		Path:      os.Getenv("OPN_PATH"),
		Email:     os.Getenv("OPN_EMAIL"),
	}

	// check if we meet basic opnsense requirements
	config.Enable = checkSetRequiredOPN()
	config.Targets = os.Getenv("OPN_TARGETS")

	// check if we meet basic unifi web-fetch backup requirements
	config.Unifi.Backup.Enable = checkSetRequiredUnifi()

	// unifi autoBackup folder watch & sync
	//
	// When OPN_UNIFI_WATCH_PATH points at the Unifi controller autoBackup
	// directory (e.g. /var/lib/unifi/data/backup/autobackup) the daemon arms a
	// goroutine that watches for filesystem change events (add/delete/rename)
	// and mirrors the newest .unf backup into the local store. The watcher is
	// only enabled when the directory exists AND contains a readable
	// autobackup_meta.json marker file (existence + read access is enough; its
	// contents are not parsed or validated); otherwise the feature is silently
	// disabled so opnborg keeps running on hosts that do not host a co-located
	// controller.
	//
	// This block runs BEFORE the minimum-requirements gate so a watch-only
	// deployment (no OPN_TARGETS, no OPN_UNIFI_WEBUI fetcher) is a valid
	// configuration: the file watcher is the sole source of backups.
	unifiWatchEnable.Store(false)
	config.Unifi.Watch.Enable = false
	config.Unifi.Watch.SetupErr = ""
	if isEnv("OPN_UNIFI_WATCH_PATH") {
		watchPath := os.Getenv("OPN_UNIFI_WATCH_PATH")
		info, err := os.Stat(watchPath)
		if err != nil {
			config.Unifi.Watch.SetupErr = "SOURCE-FOLDER-NOT-FOUND: " + err.Error()
			displayChan <- []byte("[UNIFI][WATCH][DISABLED][" + config.Unifi.Watch.SetupErr + "] " + watchPath)
		} else if !info.IsDir() {
			config.Unifi.Watch.SetupErr = "SOURCE-FOLDER-NOT-A-DIRECTORY: " + watchPath
			displayChan <- []byte("[UNIFI][WATCH][DISABLED][" + config.Unifi.Watch.SetupErr + "]")
		} else {
			metaPath := filepath.Join(watchPath, "autobackup_meta.json")
			// the marker file only needs to exist and be readable; its contents
			// are not parsed or validated, so a controller-emitted marker that
			// is not well-formed XML no longer blocks the watcher.
			if _, err := os.ReadFile(metaPath); err != nil {
				config.Unifi.Watch.SetupErr = "META-FILE-NOT-READABLE: " + err.Error()
				displayChan <- []byte("[UNIFI][WATCH][DISABLED][" + config.Unifi.Watch.SetupErr + "] " + metaPath)
			} else {
				config.Unifi.Watch.Enable = true
				config.Unifi.Watch.Path = watchPath
				config.Unifi.Watch.Meta = metaPath
				if fi, err := os.Stat(metaPath); err == nil {
					config.Unifi.Watch.LastTS = fi.ModTime()
				}
				unifiWatchEnable.Store(true)
				unifiWatchPath = watchPath
				displayChan <- []byte("[UNIFI][WATCH][ENABLED][SOURCE] " + watchPath)
			}
		}
	}

	// check if we meet basic requirements: at least one backup source (OPN
	// hive, Unifi web-fetch backup, or Unifi autoBackup folder watch) must be
	// enabled, otherwise opnborg has nothing to do.
	if !config.Enable && !config.Unifi.Backup.Enable && !config.Unifi.Watch.Enable {
		return nil, errors.New("please enable either OPN or Unifi backup, Please set OPN_TARGETS and OPN_APIKEY & OPN_APISECRET or OPN_UNIFI_WEBUI and OPN_UNIFI_BACKUP_USER & SECRET, or point OPN_UNIFI_WATCH_PATH at a co-located Unifi autoBackup folder")
	}

	// setup app name
	if config.AppName == "" {
		config.AppName = "[OPNBORG-API]"
	}

	// sanitize input
	if config.Path == "" {
		config.Path = filepath.Dir("./")
	}

	// validate bools
	config.Daemon = !isEnv("OPN_NODAEMON")
	config.Debug = isEnv("OPN_DEBUG")

	// configure backup storage git repo management (init + auto commit +
	// optional upstream SSH sync). The feature is opt-in via OPN_GIT_ENABLE;
	// when disabled the storage folder is left as a plain directory tree.
	config.Git.Enable = isEnv("OPN_GIT_ENABLE")
	config.Git.Upstream = os.Getenv("OPN_GIT_UPSTREAM")
	config.Git.SSHKey = os.Getenv("OPN_GIT_SSH_KEY")
	if err := validateGitConfig(config); err != nil {
		return nil, err
	}

	// configure remote syslog server
	config.RSysLog.Enable = false
	if config.Daemon {
		if isEnv("OPN_RSYSLOG_ENABLE") {
			if isEnv("OPN_RSYSLOG_SERVER") {
				config.RSysLog.Enable = true
				config.RSysLog.Server = os.Getenv("OPN_RSYSLOG_SERVER")
				if len(strings.Split(config.RSysLog.Server, ":")) < 2 {
					return nil, fmt.Errorf("env variable 'OPN_RSYSLOG_SERVER' format error, example \"192.168.0.100:5140\"")
				}
			}
		}
	}

	// configure httpd (disabled by default; only armed in daemon mode and only
	// when OPN_HTTPD_DISABLE is unset). Previously Enable defaulted to true,
	// which leaked an empty-address listener in one-shot mode and ignored the
	// OPN_HTTPD_DISABLE flag.
	config.Httpd.Enable = false
	if config.Daemon && !isEnv("OPN_HTTPD_DISABLE") {
		config.Httpd.Enable = true
		config.Httpd.Server = "127.0.0.1:6464"
		if isEnv("OPN_HTTPD_SERVER") {
			config.Httpd.Server = os.Getenv("OPN_HTTPD_SERVER")
			if len(strings.Split(config.Httpd.Server, ":")) < 2 {
				return nil, fmt.Errorf("env variable 'OPN_HTTPD_SERVER' format error, example \"127.0.0.1:6464\"")
			}
		}
		config.Httpd.CAcert = os.Getenv("OPN_HTTPD_CACERT")
		config.Httpd.CAkey = os.Getenv("OPN_HTTPD_CAKEY")
		config.Httpd.CAClient = os.Getenv("OPN_HTTPD_CACLIENT")
		config.Httpd.Color.FG = "white"
		config.Httpd.Color.BG = "#333333"
		if isEnv("OPN_HTTPD_COLOR_FG") {
			config.Httpd.Color.FG = os.Getenv("OPN_HTTPD_COLOR_FG")
		}
		if isEnv("OPN_HTTPD_COLOR_BG") {
			config.Httpd.Color.BG = os.Getenv("OPN_HTTPD_COLOR_BG")
		}

		var s strings.Builder
		s.WriteString("<head>" + _lf + "<title>" + _app + "</title>" + _lf)
		s.WriteString("<meta charset=\"UTF-8\">" + _lf)
		s.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">" + _lf)
		s.WriteString("<link rel=\"icon\" type=\"image/png\" href=\"favicon.ico\">" + _lf)
		css := strings.ReplaceAll(strings.ReplaceAll(_css, "%FG%", config.Httpd.Color.FG), "%BG%", config.Httpd.Color.BG)
		s.WriteString(css)
		_head = s.String() + "<meta http-equiv=\"refresh\" content=\"15\">" + _lf + "</head>" + _lf
		_headForce := s.String() + "<meta http-equiv=\"refresh\" content=\"8; url='../'\">" + _lf + "</head>" + _lf
		_forceRedirect = _htmlStart + _headForce + _bodyStart + _forceInfo + _bodyEnd + _htmlEnd
	}

	// config master
	config.Sync.Enable = false
	config.Sync.validConf = false
	config.Sync.PKG.Enable = false
	if isEnv("OPN_MASTER") {
		config.Sync.Enable = true
		config.Sync.Master = os.Getenv("OPN_MASTER")
		if _, ok := os.LookupEnv("OPN_SYNC_PKG"); ok {
			config.Sync.PKG.Enable = true
			pkghost = config.Sync.Master
			pkgmaster = "https://" + config.Sync.Master + _plug
		}
	}

	// unifi
	if config.Unifi.WebUI, err = checkURL("OPN_UNIFI_WEBUI"); err != nil {
		return config, err
	}
	s := strings.Split(os.Getenv("OPN_UNIFI_WEBUI"), "#")
	if len(s) > 1 {
		config.Unifi.Tag = s[1]
	}
	unifiBackupEnable.Store(false)
	unifiExportEnable.Store(false)
	if config.Unifi.WebUI != nil {
		unifiWebUI = config.Unifi.WebUI
		config.Unifi.Backup.Enable = false
		if _, ok := os.LookupEnv("OPN_UNIFI_BACKUP_USER"); ok {
			config.Unifi.Backup.User = os.Getenv("OPN_UNIFI_BACKUP_USER")
		}
		if _, ok := os.LookupEnv("OPN_UNIFI_BACKUP_SECRET"); ok {
			config.Unifi.Backup.Secret = os.Getenv("OPN_UNIFI_BACKUP_SECRET")
		}
		if config.Unifi.Backup.User != "" && config.Unifi.Backup.Secret != "" {
			unifiBackupEnable.Store(true)
			config.Unifi.Backup.Enable = true
			if _, ok := os.LookupEnv("OPN_UNIFI_VERSION"); !ok {
				return config, errors.New("OPN_UNIFI_VERSION must contain the unifi controller version number (eg.: '5.6.9') when unifi backup is enabled")
			}
			config.Unifi.Version = os.Getenv("OPN_UNIFI_VERSION")
			if _, ok := os.LookupEnv("OPN_UNIFI_EXPORT"); ok {
				unifiExportEnable.Store(true)
				config.Unifi.Export.Enable = true
				if config.Unifi.Export.URI, err = url.Parse("mongodb://127.0.0.1:27117"); err != nil {
					panic(err) // unreachable internal error in default mongodb uri
				}
				// Only override the default when the env var is set; checkURL
				// returns (nil, nil) when the var is absent, which would
				// otherwise wipe the default and cause a nil-pointer panic in
				// srvUnifiExport when it calls URI.String().
				if _, ok := os.LookupEnv("OPN_UNIFI_MONGODB_URI"); ok {
					if config.Unifi.Export.URI, err = checkURL("OPN_UNIFI_MONGODB_URI"); err != nil {
						return config, err
					}
				}
				config.Unifi.Export.Format = "csv"
				if d := os.Getenv("OPN_UNIFI_FORMAT"); d == "json" {
					config.Unifi.Export.Format = "json"
				}
			}
		}
	}

	//
	// WebUI Section
	//

	// prometheus
	if config.Prometheus.WebUI, err = checkURL("OPN_PROMETHEUS_WEBUI"); err != nil {
		return config, err
	}
	prometheusWebUI = config.Prometheus.WebUI

	// wazuh
	if config.Wazuh.WebUI, err = checkURL("OPN_WAZUH_WEBUI"); err != nil {
		return config, err
	}
	wazuhWebUI = config.Wazuh.WebUI

	// grafana
	if config.Grafana.WebUI, err = checkURL("OPN_GRAFANA_WEBUI"); err != nil {
		return config, err
	}
	if config.Grafana.WebUI != nil {
		grafanaWebUI = config.Grafana.WebUI
		if config.Grafana.FreeBSD, err = checkPreURL(config.Grafana.WebUI, "/d/", "OPN_GRAFANA_DASHBOARD_FREEBSD"); err != nil {
			return config, err
		}
		grafanaFreeBSD = config.Grafana.FreeBSD
		if config.Grafana.HAProxy, err = checkPreURL(config.Grafana.WebUI, "/d/", "OPN_GRAFANA_DASHBOARD_HAPROXY"); err != nil {
			return config, err
		}
		grafanaHAProxy = config.Grafana.HAProxy
		if config.Grafana.Unifi, err = checkPreURL(config.Grafana.WebUI, "/d/", "OPN_GRAFANA_DASHBOARD_UNIFI"); err != nil {
			return config, err
		}
		grafanaUnifi = config.Grafana.Unifi
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
				return nil, fmt.Errorf("env variable 'OPN_SLEEP' must contain a number in seconds without prefix or suffix")
			}
		}
		if config.Sleep < 10 {
			config.Sleep = 10
		}
		sleep = strconv.FormatInt(config.Sleep, 10)
	}
	return config, nil

}

// checkRequired OPN env
func checkSetRequiredOPN() bool {

	if !isEnv("OPN_APIKEY") || !isEnv("OPN_APISECRET") {
		return false
	}

	if isEnv("OPN_TARGETS") {
		tg = append(tg, OPNGroup{Name: "", OPN: true, Member: strings.Split(os.Getenv("OPN_TARGETS"), ",")})
		return true
	}

	env := os.Environ()
	if len(env) < 2 {
		return false
	}
	sort.Strings(env)
	var members []string
	for _, value := range env {
		name, val, found := strings.Cut(value, "=")
		if !found {
			continue
		}
		if !strings.HasPrefix(name, _opnTargetsPrefix) ||
			strings.HasPrefix(name, _opnTargetsDescPrefix) ||
			strings.HasPrefix(name, _opnTargetsImgPrefix) {
			continue
		}
		group := strings.TrimPrefix(name, _opnTargetsPrefix)
		members = append(members, val)
		desc := os.Getenv(_opnTargetsDescPrefix + group)
		imgURL := os.Getenv(_opnTargetsImgPrefix + group)
		tg = append(tg, OPNGroup{
			Name:   group,
			OPN:    true,
			Unifi:  false,
			Desc:   desc,
			ImgURL: imgURL,
			Member: strings.Split(val, ","),
		})
	}
	if len(members) > 0 {
		_ = os.Setenv("OPN_TARGETS", strings.Join(members, ","))
		return true
	}
	return false
}

// checkRequired Unifi env
func checkSetRequiredUnifi() bool {

	unifiURL, err := url.Parse(os.Getenv("OPN_UNIFI_WEBUI"))
	if err != nil {
		return false // detailed checks & err analysis later
	}

	if !isEnv("OPN_UNIFI_BACKUP_USER") || !isEnv("OPN_UNIFI_BACKUP_SECRET") {
		return false
	}

	// add unifi group
	tg = append(tg, OPNGroup{
		Name:   "UNIFI CONTROLLER",
		OPN:    false,
		Unifi:  true,
		Desc:   os.Getenv("OPN_UNIFI_BACKUP_DESC"),
		ImgURL: os.Getenv("OPN_UNIFI_BACKUP_IMGURL"),
		Member: strings.Split(unifiURL.Hostname(), ","),
	})
	return true
}
