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
	gz := gzip.NewWriter(&gzBuf)
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

var gzipTypes = []string{"text/text", "text/html", "text/plain", "text/xml", "text/css", "application/x-javascript", "application/javascript"}

//Allow ...
func Allow(contentType string) bool {
	for _, gType := range gzipTypes {
		types := strings.Split(contentType, ";")
		for _, wType := range types {
			if wType == gType {
				return true
			}
		}
	}
	return false
}
