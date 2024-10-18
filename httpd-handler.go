package opnborg

import (
	"bytes"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2/quick"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
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

// getGitHandler
func getGitHandler() http.Handler {
	h := func(r http.ResponseWriter, q *http.Request) {
		r = headHTML(r)
		switch q.Method {
		case "GET":
			compress.WriteTransportCompressedPage(getGitHTML(), r, q, true)
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
	s.WriteString(_startHTML)
	s.WriteString(_headHTML)
	s.WriteString(_bodyHTML1)
	s.WriteString(borg)
	s.WriteString(_bodyHTML2)
	s.WriteString(getNavi())
	s.WriteString(getHive())
	s.WriteString(_gitLogLink)
	s.WriteString(getPKG())
	s.WriteString(_endHTML)
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
		w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
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
	s.WriteString("<table><tr><td>")
	s.WriteString(strings.ReplaceAll(syncPKG, ",", "</td></tr>\n <tr><td>"))
	s.WriteString("</td></tr></table><br>\n")
	return s.String()
}

// getHive
func getHive() string {
	var s strings.Builder
	s.WriteString("<br><br><b>BorgHIVE</b><br><b>Module:Monitor:Backup:Active<br>[ checking state every ")
	s.WriteString(sleep)
	s.WriteString(" seconds ]</b><br><br>\n")
	s.WriteString(_lf)
	hiveMutex.Lock() // snapshot (freeze) state
	for _, grp := range tg {
		s.WriteString("<b>" + grp.Name + "</b><br>")
		s.WriteString(" <table>")
		s.WriteString(_lf)
		for _, srv := range grp.Member {
			s.WriteString("  <tr><td>")
			for _, line := range hive {
				if strings.Contains(line, srv) {
					s.WriteString(line)
					break
				}
			}
			s.WriteString("  </tr></td>")
			s.WriteString(_lf)
		}
		s.WriteString(" </table>")
		s.WriteString(" <br>")
		s.WriteString(_lf)
	}
	hiveMutex.Unlock()
	s.WriteString("<br>")
	s.WriteString(_lf)
	return s.String()
}

// getNavi provides the central top navigation links
func getNavi() string {
	var s strings.Builder
	if prometheusWebUI != "" {
		s.WriteString(" <a href=\"")
		s.WriteString(prometheusWebUI)
		s.WriteString("/targets?search=")
		s.WriteString("\" ")
		s.WriteString(_nwin)
		s.WriteString("><button><b>[ PrometheusDB ]</b></button></a> ")
	}
	if grafanaWebUI != "" {
		s.WriteString("<a href=\"")
		s.WriteString(grafanaWebUI)
		s.WriteString("/dashboards")
		s.WriteString("\" ")
		s.WriteString(_nwin)
		s.WriteString("><button><b>[ Grafana Overview ]</b></button></a> ")
	}
	if grafanaFreeBSD != "" {
		s.WriteString("<a href=\"")
		s.WriteString(grafanaWebUI)
		s.WriteString("/d/")
		s.WriteString(grafanaFreeBSD)
		s.WriteString("\" ")
		s.WriteString(_nwin)
		s.WriteString("><button><b>[ Grafana OPNSenseOS Dashboards ]</b></button></a> ")
	}
	if grafanaHAProxy != "" {
		s.WriteString("<a href=\"")
		s.WriteString(grafanaWebUI)
		s.WriteString("/d/")
		s.WriteString(grafanaHAProxy)
		s.WriteString("\" ")
		s.WriteString(_nwin)
		s.WriteString("><button><b>[ Grafana HAProxy Dashboards ]</b></button></a> ")
	}
	if wazuhWebUI != "" {
		s.WriteString(" <a href=\"")
		s.WriteString(wazuhWebUI)
		s.WriteString("/")
		s.WriteString("\" ")
		s.WriteString(_nwin)
		s.WriteString("><button><b>[ Wazuh ]</b></button></a> ")
	}
	return s.String()
}

// getGitLog ...
func getGitHTML() string {
	cgit := true
	if cgit {
		// native c lib git log
		var buf bytes.Buffer
		cmd := exec.Command("git", "log", "-c", "--since=14days")
		gitLog, err := cmd.CombinedOutput()
		if err != nil {
			_ = quick.Highlight(&buf, err.Error(), "diff", "html", "github")
			return buf.String()
		}
		_ = quick.Highlight(&buf, string(gitLog), "diff", "html", "github")
		return buf.String()
	}

	// open git repo
	repo, err := git.PlainOpen(_currentDir)
	if err != nil {
		return err.Error()
	}

	// identify repo head
	ref, err := repo.Head()
	if err != nil {
		return err.Error()
	}

	// Fetch Log
	now := time.Now()
	since := time.Now().AddDate(0, -14, 0)
	objIter, err := repo.Log(&git.LogOptions{
		From:  ref.Hash(),
		Since: &since,
		Until: &now,
	})
	if err != nil {
		return err.Error()
	}
	var s strings.Builder
	s.WriteString("<pre>")
	_ = objIter.ForEach(func(c *object.Commit) error {
		s.WriteString(_linefeed)
		s.WriteString(_linefeed)
		obj, _ := repo.CommitObject(c.Hash)
		s.WriteString(obj.String())
		s.WriteString(_linefeed)
		return nil
	})
	s.WriteString("</pre>")
	return s.String()
}
