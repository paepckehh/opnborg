package opnborg

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

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
					return nil, fmt.Errorf("env variable 'OPN_RSYSLOG_SRV' format error, example \"192.168.0.100:5140\"")
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
					return nil, fmt.Errorf("env variable 'OPN_HTTPD_SRV' format error, example \"127.0.0.1:6464\"")
				}
				config.Httpd.CAcert = os.Getenv("OPN_HTTPD_CACERT")
				config.Httpd.CAkey = os.Getenv("OPN_HTTPD_CAKEY")
				config.Httpd.CAClient = os.Getenv("OPN_HTTPD_CACLIENT")
				config.Httpd.Color.FG = "black"
				config.Httpd.Color.BG = "orange"
				if _, ok := os.LookupEnv("OPN_HTTPD_COLOR_FG"); ok {
					config.Httpd.Color.FG = os.Getenv("OPN_HTTPD_COLOR_FG")
				}
				if _, ok := os.LookupEnv("OPN_HTTPD_COLOR_BG"); ok {
					config.Httpd.Color.BG = os.Getenv("OPN_HTTPD_COLOR_BG")
				}

				var s strings.Builder
				s.WriteString("<head>" + _lf + "<title>" + _app + "</title>" + _lf)
				s.WriteString("<meta http-equiv=\"refresh\" content=\"20\">" + _lf)
				s.WriteString("<meta charset=\"UTF-8\">" + _lf)
				s.WriteString("<link rel=\"icon\" type=\"image/png\" href=\"favicon.ico\">" + _lf)
				s.WriteString("<style>body{color: ")
				s.WriteString(config.Httpd.Color.FG)
				s.WriteString(";background-color: ")
				s.WriteString(config.Httpd.Color.BG)
				s.WriteString(";}" + _lf)
				s.WriteString("</style>" + _lf + "</head>" + _lf)
				_headHTML = s.String()
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
		prometheusWebUI = config.Prometheus.WebUI
	}
	// wazuh
	if _, ok := os.LookupEnv("OPN_WAZUH_WEBUI"); ok {
		config.Wazuh.Enable = true
		config.Wazuh.WebUI = os.Getenv("OPN_WAZUH_WEBUI")
		wazuhWebUI = config.Wazuh.WebUI
	}
	// grafana
	if _, ok := os.LookupEnv("OPN_GRAFANA_WEBUI"); ok {
		config.Grafana.Enable = true
		config.Grafana.WebUI = os.Getenv("OPN_GRAFANA_WEBUI")
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
				return nil, fmt.Errorf("env variable 'OPN_SLEEP' must contain a number in seconds without prefix or suffix")
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

// checkRequired env input
func checkSetRequired() error {

	if _, ok := os.LookupEnv("OPN_APIKEY"); !ok {
		return fmt.Errorf("set env variable 'OPN_APIKEY' to your opnsense api key")
	}

	if _, ok := os.LookupEnv("OPN_APISECRET"); !ok {
		return fmt.Errorf("set env variable 'OPN_APISECRET' to your opnsense api key secret")
	}
	if _, ok := os.LookupEnv("OPN_TARGETS"); !ok {
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
						if _, ok := os.LookupEnv("OPN_TARGETS_IMGURL_" + grp[0][12:]); ok {
							tg = append(tg, OPNGroup{
								Name:   grp[0][12:],
								Img:    true,
								ImgURL: os.Getenv("OPN_TARGETS_IMGURL_" + grp[0][12:]),
								Member: strings.Split(grp[1], ","),
							})
						} else {
							tg = append(tg, OPNGroup{
								Name:   grp[0][12:],
								Img:    false,
								Member: strings.Split(grp[1], ","),
							})
						}
					}
				}
			}
			if len(member) > 0 {
				os.Setenv("OPN_TARGETS", member)
				return nil
			}
		}
		return fmt.Errorf("add at least one target server to env var 'OPN_TARGETS' or 'OPN_TARGETS_* '(multi valued, comma seperated)")
	}
	tg = append(tg, OPNGroup{Name: "Hive", Member: strings.Split(os.Getenv("OPN_TARGETS"), ",")})
	return nil
}
