package opnborg

import (
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	_ua    = "User-Agent"
	_utf8  = "text/html;charset=utf-8"
	_txt   = "text/plain"
	_ctype = "Content-Type"
	_app   = " [ -= OPNBORG =- ] "
)

// getForceHandler arms a fresh forced backup pass and returns the animated
// progress dashboard page. The dashboard polls /progress and streams the live
// log lines the backup emits, then redirects back to the hive view when the
// forced pass ends.
func getForceHandler() http.Handler {
	h := func(r http.ResponseWriter, q *http.Request) {
		// Non-blocking pokes: if a backup pass is already pending in the
		// buffered channel, drop the duplicate rather than blocking the HTTP
		// handler (and the client) for a full backup cycle.
		select {
		case updateOPN <- true:
		default:
		}
		if unifiBackupEnable.Load() {
			unifiBackupNow.Store(true)
			select {
			case updateUnifiBackup <- true:
			default:
			}
		}
		if unifiExportEnable.Load() {
			select {
			case updateUnifiExport <- true:
			default:
			}
		}
		if unifiWatchEnable.Load() {
			select {
			case updateUnifiWatch <- true:
			default:
			}
		}
		// arm a fresh forced pass so the dashboard can detect completion
		// even when a timer-tick pass was already queued in the channel.
		force := bumpForceSeq()
		r = headHTML(r)
		// inject the live forced-pass sequence into the dashboard so it can
		// tell its own pass apart from a concurrent timer-tick pass.
		_, _ = r.Write([]byte(strings.ReplaceAll(_forceRedirect, "%FORCE%", strconv.FormatUint(force, 10))))
	}
	return http.HandlerFunc(h)
}

// getFavIconHandler
func getFavIconHandler() http.Handler {
	h := func(r http.ResponseWriter, q *http.Request) {
		r.Header().Set("Content-Type", "image/png")
		r.Header().Set("Content-Length", strconv.Itoa(len(_favicon)))
		_, _ = r.Write(_favicon)
	}
	return http.HandlerFunc(h)
}

// getIndexHandler
func getIndexHandler() http.Handler {
	h := func(r http.ResponseWriter, q *http.Request) {
		r = headHTML(r)
		switch q.Method {
		case http.MethodGet:
			writeTransportCompressedPage(getStartHTML(q), r, q, true)
		default:
			inf := "Error: Method Not Allowed (405) [" + q.Method + "]"
			http.Error(r, inf, http.StatusMethodNotAllowed)
		}
	}
	return http.HandlerFunc(h)
}

// getStartHTML is the root page
func getStartHTML(q *http.Request) string {
	var s strings.Builder
	s.WriteString(_htmlStart)
	s.WriteString(_head)
	s.WriteString(_bodyStart)
	s.WriteString(getBodyHead(q))
	s.WriteString(getReviewBanner())
	s.WriteString(getNavi())
	s.WriteString(getAuditTile())
	s.WriteString(getBackupTile())
	s.WriteString(getHive())
	s.WriteString(getUnifiWatch())
	s.WriteString(getPKG())
	s.WriteString(getDashboard(_cfg))
	s.WriteString(_bodyFooter)
	s.WriteString(_bodyEnd)
	s.WriteString(_htmlEnd)
	return s.String()
}

// getReviewBanner returns a banner shown on the main page when the
// Ollama-assisted commit review is in progress. It signals to the operator
// that changes have been detected and are currently being reviewed by the AI
// model but are not yet committed to the git repo. Returns an empty string
// when no review is pending.
func getReviewBanner() string {
	if !reviewPending.Load() {
		return _empty
	}
	return "<div class=\"review-banner\"><span class=\"review-pulse\"></span><span class=\"review-text\">Changes detected &#8212; AI security review in progress, not yet committed</span></div>" + _lf
}

// headHTML
func headHTML(r http.ResponseWriter) http.ResponseWriter {
	r.Header().Set(_ctype, _utf8)
	return r
}

// addSecurityHeader stamps the baseline browser hardening headers onto every
// page-render route. The WebUI is same-origin only (no external subresources),
// so framing is denied entirely and no referrer information leaks out.
func addSecurityHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, req)
	})
}

// getPKG ...
func getPKG() string {
	syncPKG := getSyncPKG()
	if len(syncPKG) < 5 {
		return _empty
	}
	var s strings.Builder
	s.WriteString("<div class=\"backup-section sync-tile\"><b>BorgSYNC</b> <span class=\"member-meta\">[ Module:Package-Sync:Active ]</span> <span class=\"member-meta\">")
	s.WriteString(strings.ReplaceAll(strings.ReplaceAll(syncPKG, ",", " / "), "os-", ""))
	s.WriteString("</span>")
	s.WriteString("<div class=\"tile-actions\">")
	s.WriteString("<a href=\"")
	s.WriteString(pkgmaster)
	s.WriteString("\" class=\"btn btn-force\">[ Manage Plugins ]</a>")
	s.WriteString("</div>")
	s.WriteString("</div>")
	return s.String()
}

// getBackupTile renders the "BorgBACKUP" tile for the index page: a short
// heading and the live backup interval, with the "Backup NOW" trigger button
// floated to the right of the info text to save vertical space. The layout
// mirrors the BorgAUDIT tile (flex row, single line when width allows).
func getBackupTile() string {
	var s strings.Builder
	s.WriteString("<div class=\"backup-section backup-tile\"><b>BorgBACKUP</b> ")
	s.WriteString("<span class=\"member-meta\">Module:Monitor:Backup:Active [ Automatic check every ")
	s.WriteString(sleep)
	s.WriteString(" seconds ]</span>")
	s.WriteString("<div class=\"tile-actions\">")
	s.WriteString(_forceButton)
	s.WriteString("</div>")
	s.WriteString("</div>")
	return s.String()
}

// getHive
func getHive() string {
	var s strings.Builder
	hiveMutex.Lock() // snapshot (freeze) state
	for _, grp := range tg {
		s.WriteString("<div class=\"group\">")
		writeGroupHeader(&s, grp)
		for _, srv := range grp.Member {
			s.WriteString("<div class=\"member-row\">")
			writeGroupMember(&s, grp, srv)
			s.WriteString("</div>")
		}
		s.WriteString("</div>")
	}
	hiveMutex.Unlock()
	return s.String()
}

// getUnifiWatch renders the dedicated Unifi autoBackup folder-watch sync
// section. It is only emitted when the watcher was armed at Setup() time
// (config.Unifi.Watch.Enable / unifiWatchEnable). The section carries its own
// Unifi branding/logo category heading, the live sync status (green when the
// last sync succeeded), the last sync date (from the autobackup_meta.json
// marker mtime), and archive + current backup buttons, mirroring the Unifi
// backup tile.
func getUnifiWatch() string {
	if !unifiWatchEnable.Load() {
		return _empty
	}
	var s strings.Builder
	s.WriteString("<div class=\"group\">")
	s.WriteString("<div class=\"group-header\">")
	s.WriteString(_unifi)
	s.WriteString("<b>UNIFI AUTOBACKUP WATCH</b>")
	if unifiWatchPath != "" {
		s.WriteString("<span class=\"group-desc\">")
		s.WriteString(html.EscapeString(unifiWatchPath))
		s.WriteString("</span>")
	}
	s.WriteString("</div>")
	s.WriteString("<div class=\"member-row\">")
	unifiWatchMutex.Lock()
	status := unifiWatchStatus
	unifiWatchMutex.Unlock()
	if status == "" {
		status = "<div class=\"member-status\">" + _na + "</div><div class=\"member-main\"><span class=\"member-meta\">Unifi autoBackup Watch: pending Last Sync: n/a</span></div>"
	}
	s.WriteString(status)
	s.WriteString("</div>")
	s.WriteString("</div>")
	return s.String()
}

// writeGroupHeader renders the heading line for a target group.
//
// When an image URL is configured via OPN_TARGETS_IMGURL_<GROUP> (or
// OPN_UNIFI_BACKUP_IMGURL for the Unifi group) the image replaces the text
// headline. If a text description (OPN_TARGETS_DESC_<GROUP>) is also present
// it is attached as a tooltip (title attribute) on the image instead of being
// shown as a separate subheading.
//
// When no image URL is configured but a description is, the description is
// shown as a subheading beneath the group name. Otherwise only the name is shown.
func writeGroupHeader(s *strings.Builder, grp OPNGroup) {
	name := html.EscapeString(grp.Name)
	desc := html.EscapeString(grp.Desc)
	imgURL := html.EscapeString(grp.ImgURL)
	s.WriteString("<div class=\"group-header\">")
	if grp.ImgURL != "" {
		s.WriteString("<img class=\"group-img\" alt=\"")
		s.WriteString(name)
		s.WriteString("\"")
		if grp.Desc != "" {
			s.WriteString(" title=\"")
			s.WriteString(desc)
			s.WriteString("\"")
		}
		s.WriteString(" src=\"")
		s.WriteString(imgURL)
		s.WriteString("\">")
	} else {
		s.WriteString("<b>")
		s.WriteString(name)
		s.WriteString("</b>")
		if grp.Desc != "" {
			s.WriteString("<span class=\"group-desc\">")
			s.WriteString(desc)
			s.WriteString("</span>")
		}
	}
	s.WriteString("</div>")
}

// writeGroupMember renders a single hive member row, looking up the per-server
// status line for OPN groups or the shared unifi status for Unifi groups.
func writeGroupMember(s *strings.Builder, grp OPNGroup, srv string) {
	if grp.OPN {
		host, _, valid := parseServerTag(srv)
		// Guard against empty member entries (e.g. trailing comma in
		// OPN_TARGETS): an empty host would match the first hive line.
		if !valid || host == "" {
			return
		}
		for _, line := range hive {
			if hiveLineForHost(line, host) {
				s.WriteString(line)
				return
			}
		}
	}
	if grp.Unifi {
		// unifiStatus is mutated by setUnifiStatus under unifiMutex; snapshot
		// it under the same lock to avoid racing the writer goroutine.
		unifiMutex.Lock()
		status := unifiStatus
		unifiMutex.Unlock()
		s.WriteString(status)
	}
}

// hiveLineForHost reports whether a hive status line belongs to the given
// host. A plain substring match would mis-associate fw1 with the status line
// of fw10, so the match requires the host token to be delimited by non-host
// characters (or the string boundaries) on both sides. This accepts both the
// initial "Member: <host> Version:" tile and the live ">[<host>]</button>"
// rendering emitted by setOPNStatus.
func hiveLineForHost(line, host string) bool {
	token := html.EscapeString(host)
	for off := 0; off < len(line); {
		i := strings.Index(line[off:], token)
		if i < 0 {
			return false
		}
		i += off
		end := i + len(token)
		if (i == 0 || !isHostByte(line[i-1])) && (end == len(line) || !isHostByte(line[end])) {
			return true
		}
		off = end
	}
	return false
}

// isHostByte reports whether b can appear inside a host name, making it part
// of the token rather than a delimiter.
func isHostByte(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		return true
	case b == '.' || b == '-' || b == '_':
		return true
	}
	return false
}

// naviLink describes a single top-navigation entry. An empty suffix is
// allowed; the link is only emitted when the configured base URL is non-nil.
type naviLink struct {
	url    *url.URL
	suffix string
	label  string
}

// getNavi provides the central top navigation links
func getNavi() string {
	links := []naviLink{
		{url: prometheusWebUI, suffix: "/targets?search=", label: "[ PrometheusDB ]"},
		{url: grafanaWebUI, suffix: "/dashboards", label: "[ Grafana ]"},
		{url: grafanaFreeBSD, suffix: "", label: "[ OPNSense OS Dashboard ]"},
		{url: grafanaHAProxy, suffix: "", label: "[ HAProxy Dashboard ]"},
		{url: grafanaUnifi, suffix: "", label: "[ Unifi Dashboard ]"},
	}
	// Unifi controller entry is only shown when backups are NOT enabled (the
	// controller gets its own dedicated tile otherwise).
	if unifiWebUI != nil && !unifiBackupEnable.Load() {
		links = append(links, naviLink{url: unifiWebUI, suffix: "/", label: "[ Unifi ]"})
	}
	links = append(links, naviLink{url: wazuhWebUI, suffix: "/", label: "[ Wazuh ]"})

	var s strings.Builder
	s.WriteString("<nav>")
	for _, l := range links {
		if l.url == nil {
			continue
		}
		s.WriteString("<a href=\"")
		s.WriteString(html.EscapeString(l.url.String() + l.suffix))
		s.WriteString("\" ")
		s.WriteString(_nwin)
		s.WriteString("><button>")
		s.WriteString(l.label)
		s.WriteString("</button></a>")
	}
	s.WriteString("</nav>")
	return s.String()
}
