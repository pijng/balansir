package helpers

import (
	"math/rand"
	"net/http"
	"strings"
)

func RemovePortFromHost(host string) string {
	if i := strings.Index(host, ":"); i != -1 {
		host = host[:i]
	}
	return host
}

func RedirectTLS(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "https://"+RemovePortFromHost(r.Host), http.StatusMovedPermanently)
}

func RandomStringBytes(n int) string {
	const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Int63()%int64(len(letterBytes))]
	}
	return string(b)
}

func AddRemoteAddrToRequest(r *http.Request) *http.Request {
	r.Header.Set("X-Forwarded-For", r.RemoteAddr)
	return r
}
