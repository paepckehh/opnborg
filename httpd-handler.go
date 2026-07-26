package opnborg

import (
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
	_title = "title"
	_app   = " [ -= OPNBORG =- ] "
)

// getForceHandler
func getForceHandler() http.Handler {
	h := func(r http.ResponseWriter, q *http.Request) {
		updateOPN <- true
		if unifiBackupEnable.Load() {
			unifiBackupNow.Store(true)
			updateUnifiBackup <- true
		}
		if unifiExportEnable.Load() {
			unifiExportNow.Store(true)
			updateUnifiExport <- true
		}
		r = headHTML(r)
		_, _ = r.Write([]byte(_forceRedirect))
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
		case "GET":
			writeTransportCompressedPage(getStartHTML(), r, q, true)
		default:
			inf := "Error: Method Not Allowed (405) [" + q.Method + "]"
			http.Error(r, inf, http.StatusMethodNotAllowed)
		}
	}
	return http.HandlerFunc(h)
}

// getStartHTML is the root page
func getStartHTML() string {
	var s strings.Builder
	s.WriteString(_htmlStart)
	s.WriteString(_head)
	s.WriteString(_bodyStart)
	s.WriteString(_bodyHead)
	s.WriteString(getNavi())
	s.WriteString(getHive())
	s.WriteString(getPKG())
	s.WriteString(_bodySemVer)
	s.WriteString(_bodyFooter)
	s.WriteString(_bodyEnd)
	s.WriteString(_htmlEnd)
	return s.String()
}

// headHTML
func headHTML(r http.ResponseWriter) http.ResponseWriter {
	r.Header().Set(_ctype, _utf8)
	r.Header().Set(_title, _app)
	return r
}

// addSecurityHeader ...
func addSecurityHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
		// w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		next.ServeHTTP(w, req)
	})
}

// getPKG ...
func getPKG() string {
	if len(syncPKG) < 5 {
		return _empty
	}
	var s strings.Builder
	s.WriteString("<div class=\"backup-section\"><b>BorgSYNC</b> [ Module:Package-Sync:Active ]<br>")
	s.WriteString("<a href=\"")
	s.WriteString(pkgmaster)
	s.WriteString("\"><button>[ Manage Package Plugins via Master: ")
	s.WriteString(pkghost)
	s.WriteString(" ]</button></a><br><span class=\"member-meta\">")
	s.WriteString(strings.ReplaceAll(strings.ReplaceAll(syncPKG, ",", " / "), "os-", ""))
	s.WriteString("</span></div>")
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
	s.WriteString("<div class=\"backup-section\"><b>BorgBACKUP</b><br>Module:Monitor:Backup:Active<br>[ Automatic check every ")
	s.WriteString(sleep)
	s.WriteString(" seconds ]</div>")
	s.WriteString(_forceButton)
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
	s.WriteString("<div class=\"group-header\">")
	if grp.ImgURL != "" {
		s.WriteString("<img class=\"group-img\" alt=\"")
		s.WriteString(grp.Name)
		s.WriteString("\"")
		if grp.Desc != "" {
			s.WriteString(" title=\"")
			s.WriteString(grp.Desc)
			s.WriteString("\"")
		}
		s.WriteString(" src=\"")
		s.WriteString(grp.ImgURL)
		s.WriteString("\">")
	} else {
		s.WriteString("<b>")
		s.WriteString(grp.Name)
		s.WriteString("</b>")
		if grp.Desc != "" {
			s.WriteString("<span class=\"group-desc\">")
			s.WriteString(grp.Desc)
			s.WriteString("</span>")
		}
	}
	s.WriteString("</div>")
}

// writeGroupMember renders a single hive member row, looking up the per-server
// status line for OPN groups or the shared unifi status for Unifi groups.
func writeGroupMember(s *strings.Builder, grp OPNGroup, srv string) {
	if grp.OPN {
		target := strings.Split(srv, "#")
		for _, line := range hive {
			if strings.Contains(line, target[0]) {
				s.WriteString(line)
				return
			}
		}
	}
	if grp.Unifi {
		s.WriteString(unifiStatus)
	}
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
		s.WriteString(l.url.String())
		s.WriteString(l.suffix)
		s.WriteString("\" ")
		s.WriteString(_nwin)
		s.WriteString("><button>")
		s.WriteString(l.label)
		s.WriteString("</button></a>")
	}
	s.WriteString("</nav>")
	return s.String()
}
