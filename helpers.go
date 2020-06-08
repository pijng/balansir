package main

import (
	"balansir/internal/configutil"
	"balansir/internal/gziputil"
	"balansir/internal/serverutil"
	"log"
	"net"
	"net/http"
	"time"
)

func serveDistributor(endpoint *serverutil.Server, timeout int, w http.ResponseWriter, r *http.Request, gzipEnabled bool) {
	if gzipEnabled {
		gziputil.ServeWithGzip(endpoint, timeout, w, r)
		return
	}
	connection, err := net.DialTimeout("tcp", endpoint.URL.Host, time.Second*time.Duration(timeout))
	if err != nil {
		return
	}
	connection.Close()

	w = setSecureHeaders(w)
	endpoint.Proxy.ServeHTTP(w, r)
}

func setSecureHeaders(w http.ResponseWriter) http.ResponseWriter {
	w.Header().Add("X-XSS-Protection", "1; mode=block")
	w.Header().Add("X-Content-Type-Options", "nosniff")
	w.Header().Add("X-Frame-Options", "deny")
	return w
}

func setCookieToResponse(w http.ResponseWriter, hash string, configuration *configutil.Configuration) http.ResponseWriter {
	http.SetCookie(w, &http.Cookie{Name: "_balansir_server_hash", Value: hash, MaxAge: configuration.SessionMaxAge})
	return w
}

func returnPortFromHost(host string) string {
	_, host, err := net.SplitHostPort(host)
	if err != nil {
		log.Println(err)
		return ""
	}
	return host
}

func returnIPFromHost(host string) string {
	ip, _, err := net.SplitHostPort(host)
	if err != nil {
		log.Println(err)
		return ""
	}
	return ip
}

func redirectTLS(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "https://"+returnPortFromHost(r.Host), http.StatusMovedPermanently)
}

func addRemoteAddrToRequest(r *http.Request) *http.Request {
	r.Header.Set("X-Forwarded-For", r.RemoteAddr)
	return r
}
