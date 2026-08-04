package opnborg

import (
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// getConfigDashboardHandler renders a static read-only view of the OPNCall
// configuration that was parsed from the environment at Setup() time. The page
// is reachable via the "[ Config Dashboard ]" button on the main index page.
// It never exposes raw secret material (API keys, passwords, SSH keys); each
// secret-bearing field is rendered as a "set" / "not set" pill so an operator
// can verify at a glance what was understood without leaking credentials.
func getConfigDashboardHandler() http.Handler {
	h := func(r http.ResponseWriter, q *http.Request) {
		r = headHTML(r)
		switch q.Method {
		case "GET":
			writeTransportCompressedPage(getConfigDashboardHTML(), r, q, true)
		default:
			http.Error(r, "Error: Method Not Allowed (405) ["+q.Method+"]", http.StatusMethodNotAllowed)
		}
	}
	return http.HandlerFunc(h)
}

// getConfigDashboardHTML assembles the full config dashboard document.
func getConfigDashboardHTML() string {
	var s strings.Builder
	s.WriteString(_htmlStart)
	s.WriteString(_head)
	s.WriteString(_bodyStart)
	s.WriteString(_bodyHead)
	s.WriteString(getConfigNavi())
	s.WriteString(renderConfigDashboard(_cfg))
	s.WriteString(_bodySemVer)
	s.WriteString(_bodyFooter)
	s.WriteString(_bodyEnd)
	s.WriteString(_htmlEnd)
	return s.String()
}

// getConfigNavi is the top navigation for the config dashboard page. It links
// back to the main hive index and exposes the same external monitoring links as
// the index page so the dashboard is not a dead end.
func getConfigNavi() string {
	var s strings.Builder
	s.WriteString("<nav>")
	s.WriteString("<a href=\"./\"><button>[ &larr; Hive Index ]</button></a>")
	s.WriteString(getNavi())
	s.WriteString("</nav>")
	return s.String()
}

// renderConfigDashboard builds the dashboard body from the live config handle.
// A nil handle (the httpd not yet armed in tests) collapses to a placeholder.
func renderConfigDashboard(config *OPNCall) string {
	if config == nil {
		return "<div class=\"dashboard\"><h2>BorgCONFIG</h2><div class=\"dash-row\"><span class=\"dash-value dash-muted\">awaiting config</span></div></div>"
	}
	var s strings.Builder
	s.WriteString("<div class=\"dashboard\">")
	s.WriteString("<h2>BorgCONFIG &middot; Parsed Environment</h2>")
	s.WriteString("<p class=\"cfg-intro\">Snapshot of the configuration understood by opnborg at startup. Secret fields are masked.</p>")
	s.WriteString("<div class=\"dashboard-grid\">")

	s.WriteString(renderGeneralPanel(config))
	s.WriteString(renderOPNPanel(config))
	s.WriteString(renderGroupsPanel(config))
	s.WriteString(renderSyncPanel(config))
	s.WriteString(renderGitPanel(config))
	s.WriteString(renderHttpdPanel(config))
	s.WriteString(renderRSysLogPanel(config))
	s.WriteString(renderUnifiPanel(config))
	s.WriteString(renderMonitoringPanel(config))

	s.WriteString("</div></div>")
	return s.String()
}

// renderGeneralPanel covers the top-level runtime switches and identity fields.
func renderGeneralPanel(c *OPNCall) string {
	var s strings.Builder
	s.WriteString("<div class=\"dash-panel\"><div class=\"dash-title\">General</div>")
	writeDashRow(&s, "Application Name", html.EscapeString(c.AppName))
	writeDashRow(&s, "Daemon Mode", boolPill(c.Daemon))
	writeDashRow(&s, "Debug Logs", boolPill(c.Debug))
	writeDashRow(&s, "Poll Interval", strconv.FormatInt(c.Sleep, 10)+" s")
	writeDashRow(&s, "Store Path", html.EscapeString(c.Path))
	writeDashRow(&s, "Committer Email", html.EscapeString(c.Email))
	s.WriteString("</div>")
	return s.String()
}

// renderOPNPanel covers the OPNsense hive backup configuration.
func renderOPNPanel(c *OPNCall) string {
	var s strings.Builder
	s.WriteString("<div class=\"dash-panel\"><div class=\"dash-title\">OPNsense Backup</div>")
	writeDashRow(&s, "Enabled", boolPill(c.Enable))
	writeDashRow(&s, "Targets (raw)", maskIfEmpty(html.EscapeString(c.Targets)))
	writeDashRow(&s, "API Key", secretPill(c.Key))
	writeDashRow(&s, "API Secret", secretPill(c.Secret))
	writeDashRow(&s, "TLS Key Pin", secretPill(c.TLSKeyPin))
	s.WriteString("</div>")
	return s.String()
}

// renderGroupsPanel lists every parsed target group (OPN and Unifi) with its
// description, image URL, and member hosts.
func renderGroupsPanel(c *OPNCall) string {
	var s strings.Builder
	s.WriteString("<div class=\"dash-panel\"><div class=\"dash-title\">Target Groups</div>")
	if len(c.TGroups) == 0 {
		s.WriteString("<div class=\"dash-row\"><span class=\"dash-value dash-muted\">none configured</span></div>")
	} else {
		for i, grp := range c.TGroups {
			s.WriteString("<div class=\"dash-row\"><span class=\"dash-label\">Group ")
			s.WriteString(strconv.Itoa(i + 1))
			s.WriteString("</span><span class=\"dash-value\">")
			s.WriteString(html.EscapeString(groupSummary(grp)))
			s.WriteString("</span></div>")
		}
	}
	s.WriteString("</div>")
	return s.String()
}

// renderSyncPanel covers the master / package-sync feature.
func renderSyncPanel(c *OPNCall) string {
	var s strings.Builder
	s.WriteString("<div class=\"dash-panel\"><div class=\"dash-title\">Master &amp; Package Sync</div>")
	writeDashRow(&s, "Sync Enabled", boolPill(c.Sync.Enable))
	writeDashRow(&s, "Master Host", maskIfEmpty(html.EscapeString(c.Sync.Master)))
	writeDashRow(&s, "Package Sync", boolPill(c.Sync.PKG.Enable))
	if c.Sync.PKG.Enable && len(c.Sync.PKG.Packages) > 0 {
		writeDashRow(&s, "Packages", html.EscapeString(strings.Join(c.Sync.PKG.Packages, ", ")))
	}
	s.WriteString("</div>")
	return s.String()
}

// renderGitPanel covers the local-git storage management and upstream sync.
func renderGitPanel(c *OPNCall) string {
	var s strings.Builder
	s.WriteString("<div class=\"dash-panel\"><div class=\"dash-title\">Git Storage</div>")
	writeDashRow(&s, "Git Management", boolPill(c.Git.Enable))
	writeDashRow(&s, "Upstream URL", maskIfEmpty(html.EscapeString(c.Git.Upstream)))
	writeDashRow(&s, "SSH Key Path", maskIfEmpty(html.EscapeString(c.Git.SSHKey)))
	s.WriteString("</div>")
	return s.String()
}

// renderHttpdPanel covers the internal HTTP WebUI listener and TLS settings.
func renderHttpdPanel(c *OPNCall) string {
	var s strings.Builder
	s.WriteString("<div class=\"dash-panel\"><div class=\"dash-title\">Internal WebUI</div>")
	writeDashRow(&s, "Httpd Enabled", boolPill(c.Httpd.Enable))
	if c.Httpd.Enable {
		writeDashRow(&s, "Listen Address", html.EscapeString(c.Httpd.Server))
		writeDashRow(&s, "TLS Cert", maskIfEmpty(html.EscapeString(c.Httpd.CAcert)))
		writeDashRow(&s, "TLS Key", maskIfEmpty(html.EscapeString(c.Httpd.CAkey)))
		writeDashRow(&s, "mTLS Client CA", maskIfEmpty(html.EscapeString(c.Httpd.CAClient)))
		writeDashRow(&s, "Theme FG", html.EscapeString(c.Httpd.Color.FG))
		writeDashRow(&s, "Theme BG", html.EscapeString(c.Httpd.Color.BG))
	}
	s.WriteString("</div>")
	return s.String()
}

// renderRSysLogPanel covers the RFC5424 remote syslog sink.
func renderRSysLogPanel(c *OPNCall) string {
	var s strings.Builder
	s.WriteString("<div class=\"dash-panel\"><div class=\"dash-title\">Remote Syslog</div>")
	writeDashRow(&s, "Syslog Server", boolPill(c.RSysLog.Enable))
	if c.RSysLog.Enable {
		writeDashRow(&s, "Listen Address", html.EscapeString(c.RSysLog.Server))
	}
	s.WriteString("</div>")
	return s.String()
}

// renderUnifiPanel covers the Unifi controller backup, export, and autoBackup
// folder-watch features.
func renderUnifiPanel(c *OPNCall) string {
	var s strings.Builder
	s.WriteString("<div class=\"dash-panel\"><div class=\"dash-title\">Unifi</div>")
	if c.Unifi.WebUI != nil {
		writeDashRow(&s, "Controller WebUI", html.EscapeString(c.Unifi.WebUI.String()))
	} else {
		writeDashRow(&s, "Controller WebUI", "<span class=\"dash-muted\">not set</span>")
	}
	writeDashRow(&s, "Asset Tag", maskIfEmpty(html.EscapeString(c.Unifi.Tag)))
	writeDashRow(&s, "Controller Version", maskIfEmpty(html.EscapeString(c.Unifi.Version)))
	writeDashRow(&s, "Web Fetch Backup", boolPill(c.Unifi.Backup.Enable))
	if c.Unifi.Backup.Enable {
		writeDashRow(&s, "Backup User", html.EscapeString(c.Unifi.Backup.User))
		writeDashRow(&s, "Backup Secret", secretPill(c.Unifi.Backup.Secret))
	}
	writeDashRow(&s, "Inventory Export", boolPill(c.Unifi.Export.Enable))
	if c.Unifi.Export.Enable {
		writeDashRow(&s, "Export Format", html.EscapeString(c.Unifi.Export.Format))
		if c.Unifi.Export.URI != nil {
			writeDashRow(&s, "MongoDB URI", html.EscapeString(c.Unifi.Export.URI.String()))
		}
	}
	writeDashRow(&s, "AutoBackup Watch", boolPill(c.Unifi.Watch.Enable))
	if c.Unifi.Watch.Enable {
		writeDashRow(&s, "Watch Folder", html.EscapeString(c.Unifi.Watch.Path))
		writeDashRow(&s, "Meta Marker", html.EscapeString(c.Unifi.Watch.Meta))
	}
	s.WriteString("</div>")
	return s.String()
}

// renderMonitoringPanel covers the optional external monitoring dashboard links.
func renderMonitoringPanel(c *OPNCall) string {
	var s strings.Builder
	s.WriteString("<div class=\"dash-panel\"><div class=\"dash-title\">Monitoring Links</div>")
	writeDashRow(&s, "Prometheus", urlCell(c.Prometheus.WebUI))
	writeDashRow(&s, "Wazuh", urlCell(c.Wazuh.WebUI))
	writeDashRow(&s, "Grafana", urlCell(c.Grafana.WebUI))
	writeDashRow(&s, "Grafana FreeBSD", urlCell(c.Grafana.FreeBSD))
	writeDashRow(&s, "Grafana HAProxy", urlCell(c.Grafana.HAProxy))
	writeDashRow(&s, "Grafana Unifi", urlCell(c.Grafana.Unifi))
	s.WriteString("</div>")
	return s.String()
}

// writeDashRow emits a single label/value row in a dashboard panel.
func writeDashRow(s *strings.Builder, label, value string) {
	s.WriteString("<div class=\"dash-row\"><span class=\"dash-label\">")
	s.WriteString(html.EscapeString(label))
	s.WriteString("</span><span class=\"dash-value\">")
	s.WriteString(value)
	s.WriteString("</span></div>")
}

// boolPill renders a boolean as a coloured pill.
func boolPill(b bool) string {
	if b {
		return "<span class=\"dash-ok\">on</span>"
	}
	return "<span class=\"dash-muted\">off</span>"
}

// secretPill renders a "set" / "not set" pill for a secret field, never
// exposing the underlying value.
func secretPill(v string) string {
	if v == "" {
		return "<span class=\"dash-muted\">not set</span>"
	}
	return "<span class=\"dash-warn\">set (masked)</span>"
}

// maskIfEmpty renders a value or a muted "not set" placeholder when empty.
func maskIfEmpty(v string) string {
	if v == "" {
		return "<span class=\"dash-muted\">not set</span>"
	}
	return v
}

// urlCell renders a *url.URL as a clickable link, or a muted placeholder.
// A nil pointer is handled explicitly because a nil *url.URL wrapped in an
// interface is itself non-nil and would panic when String() dereferences it.
func urlCell(u *url.URL) string {
	if u == nil {
		return "<span class=\"dash-muted\">not set</span>"
	}
	return "<a href=\"" + html.EscapeString(u.String()) + "\" target=\"_blank\">" + html.EscapeString(u.String()) + "</a>"
}

// groupSummary composes a one-line summary of a target group for the panel.
func groupSummary(grp OPNGroup) string {
	var b strings.Builder
	if grp.OPN {
		b.WriteString("[OPN] ")
	}
	if grp.Unifi {
		b.WriteString("[UNIFI] ")
	}
	if grp.Name != "" {
		b.WriteString(grp.Name)
	} else {
		b.WriteString("(default)")
	}
	if grp.Desc != "" {
		b.WriteString(" — ")
		b.WriteString(grp.Desc)
	}
	b.WriteString(" | members: ")
	b.WriteString(strconv.Itoa(len(grp.Member)))
	if grp.ImgURL != "" {
		b.WriteString(" | image set")
	}
	return b.String()
}
