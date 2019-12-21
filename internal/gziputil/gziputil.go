package gziputil

import (
	"compress/gzip"
	"io"
	"log"
	"net/http"
	"strings"
)

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func (w gzipResponseWriter) WriteHeader(code int) {
	w.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(code)
}

//ServeWithGzip ...
func ServeWithGzip(fn http.HandlerFunc, w http.ResponseWriter, r *http.Request) {
	if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		fn(w, r)
		return
	}
	w.Header().Set("Content-Encoding", "gzip")
	gz := gzip.NewWriter(w)
	defer func() {
		err := gz.Close()
		if err != nil {
			log.Println("Error closing gzip writer:", err)
		}
	}()

	gzr := gzipResponseWriter{Writer: gz, ResponseWriter: w}
	fn(gzr, r)
	return
}
