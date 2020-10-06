package gziputil

import (
	"balansir/internal/logutil"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
)

//WithGzip ...
func WithGzip(r *http.Response) io.ReadCloser {
	b, _ := ioutil.ReadAll(r.Body)

	var gzBuf bytes.Buffer
	gz, _ := gzip.NewWriterLevel(&gzBuf, gzip.BestCompression)
	if _, err := gz.Write(b); err != nil {
		logutil.Error(fmt.Sprintf("Error writing to gzip: %v", err))
	}

	err := gz.Close()
	if err != nil {
		logutil.Error(fmt.Sprintf("Error closing gzip writer: %v", err))
	}

	r.Header.Set("Content-Encoding", "gzip")
	r.Header.Del("Content-Length")
	return ioutil.NopCloser(&gzBuf)
}

var gzipTypes = []string{"text/text", "text/html", "text/plain", "text/xml", "text/css", "text/javascript", "application/javascript", "application/json", "application/x-javascript", "application/xml", "application/xml+rss", "application/xhtml+xml", "application/x-font-ttf", "application/x-font-opentype", "application/vnd.ms-fontobject", "image/svg+xml", "image/x-icon", "application/rss+xml", "application/atom_xml"}

//Allow ...
func Allow(contentType string) bool {
	types := strings.Split(contentType, ";")
	for _, сType := range types {
		for _, gType := range gzipTypes {
			if сType == gType {
				return true
			}
		}
	}
	return false
}
