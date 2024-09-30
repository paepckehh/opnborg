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
	s.WriteString(getHive())
	s.WriteString(getPKG())
	s.WriteString(_gitLogLink)
	s.WriteString(_endHTML)
	return s.String()
}

// getGitHTML is the git changelog page
func getGitHTML() string {
	var s strings.Builder
	s.WriteString(_startHTML)
	s.WriteString(getGitLog())
	s.WriteString(_endHTML)
	return s.String()
}

// headHTML
func headHTML(r http.ResponseWriter) http.ResponseWriter {
	r.Header().Set(_ctype, _utf8)
	r.Header().Set(_title, _app)
	return r
}

// headSVG
func headSVG(r http.ResponseWriter) http.ResponseWriter {
	r.Header().Set(_ctype, _svg)
	r.Header().Set(_title, _app)
	return r
}

// addSecurityHeader
func addSecurityHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		next.ServeHTTP(w, req)
	})
}

// getPKG
func getPKG() string {
	if len(syncPKG) < 5 {
		return _empty
	}
	return "<br><br><b>BorgSYNC</b><br><b>Module:Package-Sync:Active</b><br>" + strings.ReplaceAll(syncPKG, ",", " ") + "<br><br>"
}

// getPKG
func getHive() string {
	return "<br><br><b>HIVE</b><br><b>Module:Backup:Active<br>[ checking state every " + sleep + " seconds ]</b><br>" + strings.Join(hive, "\n") + "<br><br>"
}

// getGitLog
func getGitLog() string {
	cgit := true
	if cgit {
		// native c lib git log
		var buf bytes.Buffer
		cmd := exec.Command("git", "log", "-c", "--since=14d")
		o, err := cmd.Output()
		if err != nil {
			_, _ = buf.WriteString("<br>GIT REPO ERROR - GIT REPO DOES NOT EXIST BEFORE FIRST SUCCESSFUL FETCH/COMMIT<br>")
			_, _ = buf.WriteString(err.Error())
		}
		_ = quick.Highlight(&buf, string(o), "diff", "html", "github")
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
		// hash := c.Hash.String()
		// line := strings.Split(c.Message, "\n")
		// s.WriteString(hash[:8])
		// s.WriteString(c.Message)
		// s.WriteString(_linefeed)
		// s.WriteString(_linefeed)
		// s.WriteString(c.String())
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
