package helpers

import (
	"balansir/internal/configutil"
	"balansir/internal/logutil"
	"balansir/internal/serverutil"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

//ReturnPortFromHost ...
func ReturnPortFromHost(host string) string {
	_, host, err := net.SplitHostPort(host)
	if err != nil {
		logutil.Warning(err)
		return ""
	}
	return host
}

//ReturnIPFromHost ...
func ReturnIPFromHost(host string) string {
	ip, _, err := net.SplitHostPort(host)
	if err != nil {
		if strings.Contains(err.Error(), "missing port in address") {
			return host
		}
		logutil.Warning(err)
	}
	return ip
}

//RedirectTLS ...
func RedirectTLS(w http.ResponseWriter, r *http.Request) {
	ip := ReturnIPFromHost(r.Host)
	TLSPort := strconv.Itoa(configutil.GetConfig().TLSPort)
	host := net.JoinHostPort(ip, TLSPort)
	target := url.URL{Scheme: "https", Host: host, Path: r.URL.Path, RawQuery: r.URL.RawQuery}
	http.Redirect(w, r, target.String(), http.StatusTemporaryRedirect)
}

//AddRemoteAddrToRequest ...
func AddRemoteAddrToRequest(r *http.Request) *http.Request {
	r.Header.Set("X-Forwarded-For", r.RemoteAddr)
	return r
}

func setSecureHeaders(w http.ResponseWriter) http.ResponseWriter {
	w.Header().Add("X-XSS-Protection", "1; mode=block")
	w.Header().Add("X-Content-Type-Options", "nosniff")
	w.Header().Add("X-Frame-Options", "deny")
	return w
}

//SetSession ...
func SetSession(w http.ResponseWriter, hash string, configuration *configutil.Configuration) http.ResponseWriter {
	http.SetCookie(w, &http.Cookie{Name: "X-Balansir-Server-Hash", Value: hash, MaxAge: configuration.SessionMaxAge})
	return w
}

//ServerPoolsEquals ...
func ServerPoolsEquals(serverPoolHash *string, incomingPool []*configutil.Endpoint) bool {
	var sumOfServerHash string
	for _, server := range incomingPool {
		serialized, _ := json.Marshal(server)
		sumOfServerHash += string(serialized)
	}
	md := md5.Sum([]byte(sumOfServerHash))
	newPoolHash := hex.EncodeToString(md[:16])
	if *serverPoolHash == newPoolHash {
		return true
	}
	*serverPoolHash = newPoolHash
	return false
}

//ServeDistributor ...
func ServeDistributor(endpoint *serverutil.Server, timeout int, w http.ResponseWriter, r *http.Request) {
	// connection, err := net.DialTimeout("tcp", endpoint.URL.Host, time.Second*time.Duration(timeout))
	// if err != nil {
	// 	return
	// }
	// connection.Close()

	w = setSecureHeaders(w)
	endpoint.Proxy.ServeHTTP(w, r)
}
