package opnborg

import (
	"errors"
	"strings"
)

// _syslogLevel is the default severity level set by opnborg on every target.
const _syslogLevel = "notice,warn,err,crit,alert,emerg"

// _syslogFacility is the default facility set by opnborg on every target.
const _syslogFacility = "kern,user,mail,daemon,auth,syslog,lpr,news,uucp,cron,authpriv,ftp,ntp,security,console,local0,local1,local2,local3,local4,local5,local6,local7"

// _syslogProgram is the default program set by opnborg on every target.
const _syslogProgram = "audit,named,configd.py,dhcpd,dhcrelay,dnsmasq,filterlog,firewall,dpinger,haproxy,charon,kea-ctrl-agent,kea-dhcp4,kea-dhcp6,lighttpd,monit,nginx,ntp,ntpd,ntpdate,openvpn,pkg,pkg-static,captiveportal,ppp,unbound,bgpd,miniupnpd,olsrd,ospfd,routed,zebra,(squid-1),suricata,wireguard,hostapd"

// _syslogDesc is the human-readable description stamped onto each destination.
const _syslogDesc = "automatic rsyslog configuration by opnborg"

// _syslogUUID is the fixed destination UUID used by opnborg.
const _syslogUUID = "ce2c4ccb-77da-4e3f-96bd-7c3fca832bc7"

// _syslogEnabled, _syslogTransport and _syslogRfc5424 are the fixed-value
// destination fields asserted by compareLogConf on every target.
const (
	_syslogEnabled   = "1"
	_syslogTransport = "udp4"
	_syslogRfc5424   = "1"
)

// checkRSysLogConfig verifies the remote syslog client configuration on the
// target server matches the opnborg-managed default.
func checkRSysLogConfig(server string, config *OPNCall, opn *Opnsense) error {
	srv := strings.Split(config.RSysLog.Server, ":")
	_ = getLogConf(srv) // ensure the configured values compile to a valid object
	return compareLogConf(server, srv, opn)
}

// compareLogConf compares the live syslog destination on the target against
// the opnborg-managed default. Each mismatch returns a labelled error so the
// caller can surface which field drifted.
func compareLogConf(server string, srv []string, opn *Opnsense) error {
	d := opn.OPNsense.Syslog.Destinations.Destination
	if d.Enabled != _syslogEnabled {
		return mismatchErr("[TARGET-REMOTE-SYSLOG-SERVER-ENABLED]", server, d.Enabled, _syslogEnabled)
	}
	if d.Transport != _syslogTransport {
		return mismatchErr("[TARGET-REMOTE-SYSLOG-TRANSPORT]", server, d.Transport, _syslogTransport)
	}
	if d.Hostname != srv[0] {
		return mismatchErr("[TARGET-REMOTE-SYSLOG-HOSTNAME]", server, d.Hostname, srv[0])
	}
	if d.Port != srv[1] {
		return mismatchErr("[TARGET-REMOTE-SYSLOG-PORT]", server, d.Port, srv[1])
	}
	if d.Rfc5424 != _syslogRfc5424 {
		return mismatchErr("[TARGET-REMOTE-SYSLOG-RFC5424]", server, d.Rfc5424, _syslogRfc5424)
	}
	return nil
}

// mismatchErr builds the labelled error string used by compareLogConf, keeping
// the historical "<label> <server> -> have: <have> need: <want>" diagnostic
// format intact while removing the inline duplication.
func mismatchErr(label, server, have, want string) error {
	return errors.New(label + " " + server + " -> have: " + have + " need: " + want)
}

// getLogConf return an OPNSense RSysLog Configuration Object
func getLogConf(srv []string) *Opnsense {
	opn := new(Opnsense)
	d := &opn.OPNsense.Syslog.Destinations.Destination
	d.Uuid = _syslogUUID
	d.Enabled = _syslogEnabled
	d.Transport = _syslogTransport
	d.Level = _syslogLevel
	d.Hostname = srv[0]
	d.Port = srv[1]
	d.Certificate = ""
	d.Rfc5424 = _syslogRfc5424
	d.Description = _syslogDesc
	d.Facility = _syslogFacility
	d.Program = _syslogProgram
	return opn
}
