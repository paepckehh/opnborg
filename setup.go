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
)

// global
var (
	hive                  []string
	hiveMutex, unifiMutex sync.Mutex
	updateOPN             = make(chan bool, 1)
	updateUnifiBackup     = make(chan bool, 1)
	updateUnifiExport     = make(chan bool, 1)
	unifiStatus           string
)

// Setup reads OPNBorgs configuration via env, sanitizes, sets sane defaults
func Setup() (*OPNCall, error) {

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

	// check if we meet basic requirements
	config.Unifi.Backup.Enable = checkSetRequiredUnifi()
	if !config.Enable && !config.Unifi.Backup.Enable {
		return nil, errors.New("Please enable either OPN or Unifi backup. Please set OPN_APIKEY & OPN_APISECRET or OPN_UNIFI_BACKUP_USER & SECRET")
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
	config.Git = !isEnv("OPN_NOGIT")
	config.GitPush = isEnv("OPN_GITPUSH")

	// configure remote syslog server
	config.RSysLog.Enable = false
	if config.Daemon {
		if isEnv("OPN_RSYSLOG_ENABLE") {
			if isEnv("OPN_RSYSLOG_SERVER") {
				config.RSysLog.Enable = true
				config.RSysLog.Server = os.Getenv("OPN_RSYSLOG_SERVER")
				if len(strings.Split(config.RSysLog.Server, ":")) < 1 {
					return nil, fmt.Errorf("env variable 'OPN_RSYSLOG_SRV' format error, example \"192.168.0.100:5140\"")
				}
			}
		}
	}

	// configure httpd
	config.Httpd.Enable = true
	if config.Daemon {
		if !isEnv("OPN_HTTPD_DISABLE") {
			config.Httpd.Enable = true
			config.Httpd.Server = "127.0.0.1:6464"
			if isEnv("OPN_HTTPD_SERVER") {
				config.Httpd.Server = os.Getenv("OPN_HTTPD_SERVER")
				if len(strings.Split(config.Httpd.Server, ":")) < 1 {
					return nil, fmt.Errorf("env variable 'OPN_HTTPD_SRV' format error, example \"127.0.0.1:6464\"")
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
			s.WriteString(" <style>" + _lf)
			s.WriteString("  table,th,td{border: 1px solid " + config.Httpd.Color.FG + "; border-collapse: collapse; padding: 8px;}" + _lf)
			s.WriteString("  body{font-family:sans-serif;color: " + config.Httpd.Color.FG + ";background-color: " + config.Httpd.Color.BG + ";}" + _lf)
			s.WriteString(" </style>" + _lf)
			_head = s.String() + "<meta http-equiv=\"refresh\" contenti=\"15\">" + _lf + "</head>" + _lf
			_headForce := s.String() + "<meta http-equiv=\"refresh\" content=\"8; url='../'\">" + _lf + "</head>" + _lf
			_forceRedirect = _htmlStart + _headForce + _bodyStart + _forceInfo + _bodyEnd + _htmlEnd

		}
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
				if config.Unifi.Export.URI, err = checkURL("OPN_UNIFI_MONGODB_URI"); err != nil {
					return config, err
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
		tg = append(tg, OPNGroup{Name: "", Img: false, OPN: true, Member: strings.Split(os.Getenv("OPN_TARGETS"), ",")})
		return true
	}

	member := ""
	env := os.Environ()
	if len(env) > 1 {
		sort.Strings(env)
		for _, value := range env {
			if len(value) > 15 {
				if value[0:12] == "OPN_TARGETS_" {
					if value[0:18] == "OPN_TARGETS_IMGURL" {
						continue
					}
					grp := strings.Split(value, "=")
					if len(member) > 0 {
						member = member + ","
					}
					member = member + grp[1]
					if isEnv("OPN_TARGETS_IMGURL_" + grp[0][12:]) {
						tg = append(tg, OPNGroup{
							Name:   grp[0][12:],
							Img:    true,
							OPN:    true,
							Unifi:  false,
							ImgURL: os.Getenv("OPN_TARGETS_IMGURL_" + grp[0][12:]),
							Member: strings.Split(grp[1], ","),
						})
					} else {
						tg = append(tg, OPNGroup{
							Name:   grp[0][12:],
							Img:    false,
							OPN:    true,
							Unifi:  false,
							Member: strings.Split(grp[1], ","),
						})
					}
				}
			}
		}
		if len(member) > 0 {
			os.Setenv("OPN_TARGETS", member)
			return true
		}
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
	if isEnv("OPN_UNIFI_BACKUP_IMGURL") {
		tg = append(tg, OPNGroup{
			Name:   "UNIFI CONTROLLER",
			Img:    true,
			OPN:    false,
			Unifi:  true,
			ImgURL: os.Getenv("OPN_UNIFI_BACKUP_IMGURL"),
			Member: strings.Split(unifiURL.Hostname(), ","),
		})
	} else {
		tg = append(tg, OPNGroup{
			Name:   "UNIFI CONTROLLER",
			Img:    false,
			OPN:    false,
			Unifi:  true,
			Member: strings.Split(unifiURL.Hostname(), ","),
		})
	}
	return true
}
