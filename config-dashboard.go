package opnborg

import (
	"html"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
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
	s.WriteString(renderRawEnvSection())
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
	writeDashRow(&s, "Targets (raw)", maskIfEmpty(formatTargetsDisplay(c.Targets)))
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
			if len(grp.Member) > 0 {
				s.WriteString("<div class=\"dash-row\"><span class=\"dash-label\">Members</span><span class=\"dash-value\">")
				s.WriteString(formatTargetsDisplay(strings.Join(grp.Member, ",")))
				s.WriteString("</span></div>")
			}
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
	if c.Unifi.Watch.SetupErr != "" {
		writeDashRow(&s, "Setup Error", "<span class=\"dash-err\">"+html.EscapeString(c.Unifi.Watch.SetupErr)+"</span>")
	}
	if c.Unifi.Watch.Enable {
		writeDashRow(&s, "Watch Folder", html.EscapeString(c.Unifi.Watch.Path))
		writeDashRow(&s, "Meta Marker", html.EscapeString(c.Unifi.Watch.Meta))
	}
	// runtime sync stats (guarded by unifiWatchMutex in srvUnifiWatch.go)
	unifiWatchMutex.Lock()
	lastSyncTS := c.Unifi.Watch.LastSyncTS
	lastSyncErr := c.Unifi.Watch.LastSyncErr
	sourceFiles := c.Unifi.Watch.SourceFiles
	syncedFiles := c.Unifi.Watch.SyncedFiles
	skippedFiles := c.Unifi.Watch.SkippedFiles
	lastFile := c.Unifi.Watch.LastFile
	unifiWatchMutex.Unlock()
	if c.Unifi.Watch.Enable {
		writeDashRow(&s, "Source Files", strconv.Itoa(sourceFiles))
		writeDashRow(&s, "Last Synced", strconv.Itoa(syncedFiles))
		writeDashRow(&s, "Skipped (dup)", strconv.Itoa(skippedFiles))
		if lastFile != "" {
			writeDashRow(&s, "Last File", html.EscapeString(lastFile))
		}
		lastSync := "n/a"
		if !lastSyncTS.IsZero() {
			lastSync = lastSyncTS.UTC().Format(time.RFC3339)
		}
		writeDashRow(&s, "Last Sync Pass", lastSync)
		if lastSyncErr != "" {
			writeDashRow(&s, "Sync Error", "<span class=\"dash-err\">"+html.EscapeString(lastSyncErr)+"</span>")
		} else if c.Unifi.Watch.Enable && !lastSyncTS.IsZero() {
			writeDashRow(&s, "Sync State", "<span class=\"dash-ok\">ok</span>")
		}
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

// formatTargetsDisplay renders a comma-separated target list (OPN_TARGETS or an
// OPN_TARGETS_<GROUP> value) with '#' as the target unit separator, as used on
// the config dashboard and the raw environment output. Empty entries produced
// by trailing/doubled commas are dropped.
// formatTargetsDisplay renders a comma-separated target list (OPN_TARGETS or an
// OPN_TARGETS_<GROUP> value) as one styled chip per target unit, so the host and
// its optional '#<asset-tag>' suffix stay visually grouped and the unit
// boundaries are unambiguous (joining units with '#' would collide with the
// in-unit '#tag' separator, e.g. "opn01.lan#RACK-PROD01#opn02.lan#RACK-PROD02").
// Empty entries from trailing/doubled commas are dropped; whitespace is trimmed.
// The returned string is already HTML-safe and must NOT be passed through
// html.EscapeString again.
func formatTargetsDisplay(raw string) string {
	parts := strings.Split(raw, ",")
	var b strings.Builder
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		b.WriteString("<span class=\"target-chip\">")
		b.WriteString(html.EscapeString(p))
		b.WriteString("</span>")
	}
	return b.String()
}

// isTargetEnvName reports whether name is a target-list env var whose value is
// a comma-separated list of target units (OPN_TARGETS or OPN_TARGETS_<GROUP>,
// excluding the _DESC_ and _IMGURL_ variants).
func isTargetEnvName(name string) bool {
	if name == "OPN_TARGETS" {
		return true
	}
	if strings.HasPrefix(name, _opnTargetsPrefix) &&
		!strings.HasPrefix(name, _opnTargetsDescPrefix) &&
		!strings.HasPrefix(name, _opnTargetsImgPrefix) {
		return true
	}
	return false
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

// _rawEnvNames is the fixed list of OPN_* environment variables recognised by
// opnborg's Setup(). Variables that are NOT in this list but carry the OPN_
// prefix are surfaced in a separate "unrecognised" subsection so an operator can
// spot typos or stale config. Group-scoped vars (OPN_TARGETS_<GROUP>,
// OPN_TARGETS_DESC_<GROUP>, OPN_TARGETS_IMGURL_<GROUP>) are matched by prefix.
var _rawEnvNames = []string{
	"OPN_APIKEY",
	"OPN_APISECRET",
	"OPN_TLSKEYPIN",
	"OPN_PATH",
	"OPN_EMAIL",
	"OPN_TARGETS",
	"OPN_NODAEMON",
	"OPN_DEBUG",
	"OPN_GIT_ENABLE",
	"OPN_GIT_UPSTREAM",
	"OPN_GIT_SSH_KEY",
	"OPN_RSYSLOG_ENABLE",
	"OPN_RSYSLOG_SERVER",
	"OPN_HTTPD_DISABLE",
	"OPN_HTTPD_SERVER",
	"OPN_HTTPD_CACERT",
	"OPN_HTTPD_CAKEY",
	"OPN_HTTPD_CACLIENT",
	"OPN_HTTPD_COLOR_FG",
	"OPN_HTTPD_COLOR_BG",
	"OPN_MASTER",
	"OPN_SYNC_PKG",
	"OPN_UNIFI_WEBUI",
	"OPN_UNIFI_BACKUP_USER",
	"OPN_UNIFI_BACKUP_SECRET",
	"OPN_UNIFI_BACKUP_DESC",
	"OPN_UNIFI_BACKUP_IMGURL",
	"OPN_UNIFI_VERSION",
	"OPN_UNIFI_EXPORT",
	"OPN_UNIFI_MONGODB_URI",
	"OPN_UNIFI_FORMAT",
	"OPN_UNIFI_WATCH_PATH",
	"OPN_PROMETHEUS_WEBUI",
	"OPN_WAZUH_WEBUI",
	"OPN_GRAFANA_WEBUI",
	"OPN_GRAFANA_DASHBOARD_FREEBSD",
	"OPN_GRAFANA_DASHBOARD_HAPROXY",
	"OPN_GRAFANA_DASHBOARD_UNIFI",
	"OPN_SLEEP",
}

// _rawEnvSecrets are env vars whose values must never be echoed verbatim. They
// are rendered as a "set (masked)" pill instead.
var _rawEnvSecrets = map[string]bool{
	"OPN_APIKEY":              true,
	"OPN_APISECRET":           true,
	"OPN_TLSKEYPIN":           true,
	"OPN_GIT_SSH_KEY":         true,
	"OPN_UNIFI_BACKUP_SECRET": true,
	"OPN_HTTPD_CAKEY":         true,
}

// _rawEnvPrefixGroups are OPN_ variable prefixes that define an open family of
// recognised group-scoped variables (the suffix is a free-form group name).
var _rawEnvPrefixGroups = []string{
	"OPN_TARGETS_",
	"OPN_TARGETS_DESC_",
	"OPN_TARGETS_IMGURL_",
}

// renderRawEnvSection emits a full-width "Raw Environment" section that lists
// every recognised OPN_* env var (whether set or not) followed by any other
// OPN_-prefixed vars present in the process environment that opnborg does NOT
// recognise. Secret-bearing vars are masked. The list is sourced from the live
// process environment at render time so it always reflects what the daemon
// actually saw.
func renderRawEnvSection() string {
	live := make(map[string]string)
	for _, kv := range os.Environ() {
		name, val, found := strings.Cut(kv, "=")
		if !found {
			continue
		}
		live[name] = val
	}

	// Build the recognised list: fixed names plus group-scoped vars present in
	// the environment. Group-scoped vars are only shown when actually set (the
	// suffix space is unbounded).
	recognised := make([]string, 0, len(_rawEnvNames)+8)
	recognised = append(recognised, _rawEnvNames...)
	for name := range live {
		if !strings.HasPrefix(name, "OPN_") {
			continue
		}
		for _, pfx := range _rawEnvPrefixGroups {
			if strings.HasPrefix(name, pfx) {
				recognised = append(recognised, name)
				break
			}
		}
	}
	sort.Strings(recognised)

	// Determine which live OPN_ vars were NOT recognised (typos / stale).
	recognisedSet := make(map[string]bool, len(recognised))
	for _, n := range recognised {
		recognisedSet[n] = true
	}
	var unknown []string
	for name := range live {
		if !strings.HasPrefix(name, "OPN_") {
			continue
		}
		if !recognisedSet[name] {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(unknown)

	var s strings.Builder
	s.WriteString("<div class=\"dashboard\">")
	s.WriteString("<h2>BorgCONFIG &middot; Raw Environment</h2>")
	s.WriteString("<p class=\"cfg-intro\">Every OPN_* env var recognised by opnborg, with the raw value as seen by the process. Secret-bearing vars are masked. Unset vars are shown as <span class=\"dash-muted\">not set</span>.</p>")
	s.WriteString("<div class=\"raw-env-grid\">")
	for _, name := range recognised {
		s.WriteString("<div class=\"raw-env-row\"><span class=\"raw-env-name\">")
		s.WriteString(html.EscapeString(name))
		s.WriteString("</span><span class=\"raw-env-val\">")
		s.WriteString(renderRawEnvValue(name, live[name]))
		s.WriteString("</span></div>")
	}
	s.WriteString("</div>")

	if len(unknown) > 0 {
		s.WriteString("<h3 class=\"raw-env-sub\">Unrecognised OPN_* vars present in environment</h3>")
		s.WriteString("<p class=\"cfg-intro\">These OPN_-prefixed vars are set but not consumed by opnborg; likely typos or stale config.</p>")
		s.WriteString("<div class=\"raw-env-grid raw-env-unknown\">")
		for _, name := range unknown {
			s.WriteString("<div class=\"raw-env-row\"><span class=\"raw-env-name\">")
			s.WriteString(html.EscapeString(name))
			s.WriteString("</span><span class=\"raw-env-val\">")
			s.WriteString(renderRawEnvValue(name, live[name]))
			s.WriteString("</span></div>")
		}
		s.WriteString("</div>")
	}

	s.WriteString("</div>")
	return s.String()
}

// renderRawEnvValue formats a single env var's value, masking secrets and
// rendering empty values as a muted "not set" pill.
func renderRawEnvValue(name, val string) string {
	if val == "" {
		return "<span class=\"dash-muted\">not set</span>"
	}
	if _rawEnvSecrets[name] {
		return "<span class=\"dash-warn\">set (masked)</span>"
	}
	if isTargetEnvName(name) {
		return formatTargetsDisplay(val)
	}
	return "<code>" + html.EscapeString(val) + "</code>"
}
