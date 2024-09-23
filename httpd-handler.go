package opnborg

import (
	"net/http"
	"strings"

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

// getStartHTML is the root pacge
func getStartHTML() string {
	var s strings.Builder
	s.WriteString(_root)
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
