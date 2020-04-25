package gziputil

import (
	"balansir/internal/serverutil"
	"compress/gzip"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
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
func ServeWithGzip(endpoint *serverutil.Server, timeout int, w http.ResponseWriter, r *http.Request) {
	if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		connection, err := net.DialTimeout("tcp", endpoint.URL.Host, time.Second*time.Duration(timeout))
		if err != nil {
			return
		}
		connection.Close()
		endpoint.Proxy.ServeHTTP(w, r)
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
	connection, err := net.DialTimeout("tcp", endpoint.URL.Host, time.Second*time.Duration(timeout))
	if err != nil {
		return
	}
	connection.Close()
	endpoint.Proxy.ServeHTTP(gzr, r)
}
