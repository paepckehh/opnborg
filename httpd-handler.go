package opnborg

import (
	"net/http"
	"strconv"
	"strings"

	"paepcke.de/npad/compress"
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
		if unifiEnable.Load() {
			unifiBackupNow.Store(true)
			updateUnifi <- true
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
			compress.WriteTransportCompressedPage(getStartHTML(), r, q, true)
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
	s.WriteString(borg)
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
	s.WriteString("<br><br><b>BorgSYNC</b><br><b> [ Module:Package-Sync:Active ] </b><br>\n")
	s.WriteString("<a href=\"" + pkgmaster + "\"><Button><b> [ Manage Package Plugin Master ] </b></Button></a><br><br>")
	s.WriteString("<table><tr><td><small>")
	s.WriteString(strings.ReplaceAll(strings.ReplaceAll(syncPKG, ",", " / "), "os-", ""))
	s.WriteString("</small></td></tr></table>")
	s.WriteString("<br>\n")
	return s.String()
}

// getHive
func getHive() string {
	var s strings.Builder
	s.WriteString("<br><br><br>")
	hiveMutex.Lock() // snapshot (freeze) state
	for _, grp := range tg {
		if grp.Img {
			s.WriteString("<b><img alt=\"" + grp.Name + "\" src=\"" + grp.ImgURL + "\"></b><br>")
		} else {
			s.WriteString("<b>" + grp.Name + "</b><br>")
		}
		s.WriteString(" <table>")
		s.WriteString(_lf)
		for _, srv := range grp.Member {
			s.WriteString("  <tr><td>")
			if grp.OPN {
				for _, line := range hive {
					target := strings.Split(srv, "#")
					if strings.Contains(line, target[0]) {
						s.WriteString(line)
						break
					}
				}
			}
			if grp.Unifi {
				s.WriteString(unifiStatus)
			}
			s.WriteString("  </td></tr>")
			s.WriteString(_lf)
		}
		s.WriteString(" </table>")
		s.WriteString(" <br>")
		s.WriteString(_lf)
	}
	hiveMutex.Unlock()
	s.WriteString(_lf)
	s.WriteString("<b>BorgBACKUP</b><br><b>Module:Monitor:Backup:Active<br>[ Automatic check every ")
	s.WriteString(sleep)
	s.WriteString(" seconds ]</b><br>" + _lf)
	s.WriteString(_forceButton + "<br><br>" + _lf)
	return s.String()
}

// getNavi provides the central top navigation links
func getNavi() string {
	var s strings.Builder
	if prometheusWebUI != nil {
		s.WriteString(" <a href=\"")
		s.WriteString(prometheusWebUI.String())
		s.WriteString("/targets?search=")
		s.WriteString("\" ")
		s.WriteString(_nwin)
		s.WriteString("><button><b>[ PrometheusDB ]</b></button></a> ")
	}
	if grafanaWebUI != nil {
		s.WriteString("<a href=\"")
		s.WriteString(grafanaWebUI.String())
		s.WriteString("/dashboards")
		s.WriteString("\" ")
		s.WriteString(_nwin)
		s.WriteString("><button><b>[ Grafana ]</b></button></a> ")
	}
	if grafanaFreeBSD != nil {
		s.WriteString("<a href=\"")
		s.WriteString(grafanaFreeBSD.String())
		s.WriteString("\" ")
		s.WriteString(_nwin)
		s.WriteString("><button><b>[ OPNSense OS Dashboard ]</b></button></a> ")
	}
	if grafanaHAProxy != nil {
		s.WriteString("<a href=\"")
		s.WriteString(grafanaHAProxy.String())
		s.WriteString("\" ")
		s.WriteString(_nwin)
		s.WriteString("><button><b>[ HAProxy Dashboard ]</b></button></a> ")
	}
	if grafanaUnifi != nil {
		s.WriteString("<a href=\"")
		s.WriteString(grafanaUnifi.String())
		s.WriteString("\" ")
		s.WriteString(_nwin)
		s.WriteString("><button><b>[ Unifi Dashboard ]</b></button></a> ")
	}
	if unifiWebUI != nil && !unifiEnable.Load() {
		s.WriteString(" <a href=\"")
		s.WriteString(unifiWebUI.String())
		s.WriteString("/")
		s.WriteString("\" ")
		s.WriteString(_nwin)
		s.WriteString("><button><b>[ Unifi ]</b></button></a> ")
	}
	if wazuhWebUI != nil {
		s.WriteString(" <a href=\"")
		s.WriteString(wazuhWebUI.String())
		s.WriteString("/")
		s.WriteString("\" ")
		s.WriteString(_nwin)
		s.WriteString("><button><b>[ Wazuh ]</b></button></a> ")
	}
	return s.String()
}
