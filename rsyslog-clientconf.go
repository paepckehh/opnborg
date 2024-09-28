package opnborg

import (
	"errors"
	"strings"
)

// checkRSysLogConfig
func checkRSysLogConfig(server string, config *OPNCall, opn *Opnsense) error {

	srv := strings.Split(config.RSysLog.Server, ":")

	// get target configuration
	_ = getLogConf(srv)

	// compare
	if opn.OPNsense.Syslog.Destinations.Destination.Hostname != srv[0] {
		details := server + " -> have: " + opn.OPNsense.Syslog.Destinations.Destination.Hostname + " need: " + srv[0]
		return errors.New("[RSYSLOG-CONF][FAIL][TARGET-SYSLOG-SERVER-HOSTNAME] " + details)
	}
	if opn.OPNsense.Syslog.Destinations.Destination.Port != srv[1] {
		details := server + " -> have: " + opn.OPNsense.Syslog.Destinations.Destination.Port + " need: " + srv[1]
		return errors.New("[RSYSLOG-CONF][FAIL][TARGET-SYSLOG-SERVER-PORT] " + details)
	}
	return nil
}

// getLogConf return an OPNSense RSysLog Configuration Object
func getLogConf(srv []string) *Opnsense {
	opn := new(Opnsense)
	opn.OPNsense.Syslog.Destinations.Destination.Uuid = "ce2c4ccb-77da-4e3f-96bd-7c3fca832bc7"
	opn.OPNsense.Syslog.Destinations.Destination.Enabled = "1"
	opn.OPNsense.Syslog.Destinations.Destination.Transport = "udp4"
	opn.OPNsense.Syslog.Destinations.Destination.Level = "notice,warn,err,crit,alert,emerg"
	opn.OPNsense.Syslog.Destinations.Destination.Hostname = srv[0]
	opn.OPNsense.Syslog.Destinations.Destination.Port = srv[1]
	opn.OPNsense.Syslog.Destinations.Destination.Certificate = ""
	opn.OPNsense.Syslog.Destinations.Destination.Rfc5424 = "1"
	opn.OPNsense.Syslog.Destinations.Destination.Description = "automatic rsyslog configuration by opnborg"
	opn.OPNsense.Syslog.Destinations.Destination.Facility = "kern,user,mail,daemon,auth,syslog,lpr,news,uucp,cron,authpriv,ftp,ntp,security,console,local0,local1,local2,local3,local4,local5,local6,local7"
	opn.OPNsense.Syslog.Destinations.Destination.Program = "audit,named,configd.py,dhcpd,dhcrelay,dnsmasq,filterlog,firewall,dpinger,haproxy,charon,kea-ctrl-agent,kea-dhcp4,kea-dhcp6,lighttpd,monit,nginx,ntp,ntpd,ntpdate,openvpn,pkg,pkg-static,captiveportal,ppp,unbound,bgpd,miniupnpd,olsrd,ospfd,routed,zebra,(squid-1),suricata,wireguard,hostapd"
	return opn
}
