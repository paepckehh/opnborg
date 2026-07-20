package opnborg

// import
import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// _compressMTU is the threshold below which compression is skipped because the
// payload already fits into a single TCP frame (MTU ~1400 bytes).
const _compressMTU = 1400

// writeTransportCompressedPage renders a page to the HTTP transport and, when
// the payload is large enough to span multiple TCP frames, transparently
// negotiates a per-connection compression scheme based on the client's
// Accept-Encoding header. Supported schemes are gzip and deflate (zlib), both
// produced with the strongest level offered by the standard library. This is
// the internal equivalent of the former paepcke.de/npad/compress
// WriteTransportCompressedPage helper, scoped down to the encodings the
// standard library can produce without third-party dependencies.
func writeTransportCompressedPage(page string, r http.ResponseWriter, q *http.Request, tryCompress bool) {
	p := []byte(page)
	var err error
	if tryCompress && len(page) > _compressMTU {
		accept := strings.Join(q.Header["Accept-Encoding"], " ")
		switch {
		case strings.Contains(accept, "gzip"):
			r.Header().Set("Content-Encoding", "gzip")
			p = compressLevel(p, gzip.NewWriterLevel)
		case strings.Contains(accept, "deflate"):
			r.Header().Set("Content-Encoding", "deflate")
			p = compressLevel(p, zlib.NewWriterLevel)
		}
		_, err = fmt.Fprint(r, string(p))
	} else {
		_, err = fmt.Fprint(r, page)
	}
	if err != nil {
		displayChan <- []byte("[HTTPD][COMPRESS][FAIL] " + err.Error())
	}
}

// levelWriter is the interface satisfied by both *gzip.Writer and *zlib.Writer.
type levelWriter interface {
	Write([]byte) (int, error)
	Close() error
}

// compressLevel encodes data at the maximum standard-library compression level
// using the supplied writer constructor (gzip.NewWriterLevel or
// zlib.NewWriterLevel). On any internal error it reports via displayChan and
// returns the original data unchanged so the caller can still serve the page.
func compressLevel[W levelWriter](data []byte, newWriter func(io.Writer, int) (W, error)) []byte {
	var buf bytes.Buffer
	w, err := newWriter(&buf, gzip.BestCompression)
	if err != nil {
		displayChan <- []byte("[HTTPD][COMPRESS][FAIL] " + err.Error())
		return data
	}
	if _, err := w.Write(data); err != nil {
		displayChan <- []byte("[HTTPD][COMPRESS][FAIL] " + err.Error())
		return data
	}
	if err := w.Close(); err != nil {
		displayChan <- []byte("[HTTPD][COMPRESS][FAIL] " + err.Error())
		return data
	}
	return buf.Bytes()
}
