package opnborg

import (
	"bytes"
	"net/http"
	"os/exec"
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
	_svg   = "image/svg+xml"
	_ctype = "Content-Type"
	_title = "Title"
	_app   = "OPNBORG"
)

// getIconHandler
func getIconHandler() http.Handler {
	h := func(r http.ResponseWriter, q *http.Request) {
		r = headSVG(r)
		compress.WriteTransportCompressedPage(_icon, r, q, true)
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
	s.WriteString(_bodyHTML)
	s.WriteString(getNavi())
	s.WriteString(getHive())
	s.WriteString(getPKG())
	s.WriteString(_gitLogLink)
	s.WriteString(_endHTML)
	return s.String()
}

// headHTML
func headHTML(r http.ResponseWriter) http.ResponseWriter {
	r.Header().Set(_ctype, _utf8)
	r.Header().Set(_title, _app)
	return r
}

// headSVG ...
func headSVG(r http.ResponseWriter) http.ResponseWriter {
	r.Header().Set(_ctype, _svg)
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
	s.WriteString("<br><br><b>BorgSYNC</b><br><b>Module:Package-Sync:Active</b><br>")
	s.WriteString(strings.ReplaceAll(syncPKG, ",", " "))
	s.WriteString("<br><br>")
	return s.String()
}

// getHive ...
func getHive() string {
	var s strings.Builder
	s.WriteString("<br><br><b>HIVE</b><br><b>Module:Backup:Active<br>[ checking state every ")
	s.WriteString(sleep)
	s.WriteString(" seconds ]</b><br>")
	s.WriteString(strings.Join(hive, "\n"))
	s.WriteString("<br><br>")
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
		s.WriteString("><button type=\"button\"><b>[ PrometheusDB ]</b></button></a> ")
	}
	if grafanaWebUI != "" {
		s.WriteString("<a href=\"")
		s.WriteString(grafanaWebUI)
		s.WriteString("/dashboards")
		s.WriteString("\" ")
		s.WriteString(_nwin)
		s.WriteString("><button type=\"button\"><b>[ Grafana Overview ]</b></button></a> ")
	}
	if grafanaFreeBSD != "" {
		s.WriteString("<a href=\"")
		s.WriteString(grafanaWebUI)
		s.WriteString("/d/")
		s.WriteString(grafanaFreeBSD)
		s.WriteString("\" ")
		s.WriteString(_nwin)
		s.WriteString("><button type=\"button\"><b>[ Grafana OPNSenseOS Dashboards ]</b></button></a> ")
	}
	if grafanaHAProxy != "" {
		s.WriteString("<a href=\"")
		s.WriteString(grafanaWebUI)
		s.WriteString("/d/")
		s.WriteString(grafanaHAProxy)
		s.WriteString("\" ")
		s.WriteString(_nwin)
		s.WriteString("><button type=\"button\"><b>[ Grafana HAProxy Dashboards ]</b></button></a> ")
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
