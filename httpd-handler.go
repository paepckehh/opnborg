package opnborg

import (
	"net/http"
	"strings"

	"paepcke.de/npad/compress"
)

// getIconHandler
func getIconHandler() http.Handler {
	h := func(r http.ResponseWriter, q *http.Request) {
		r = _headSVG(r)
		compress.WriteTransportCompressedPage(_icon, r, q, true)
	}
	return http.HandlerFunc(h)
}

// getIndexHandler
func getIndexHandler() http.Handler {
	h := func(r http.ResponseWriter, q *http.Request) {
		r = _headHTML(r)
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
