package helpers

import (
	"balansir/internal/confg"
	"balansir/internal/gziputil"
	"balansir/internal/serverutil"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

//ReturnPortFromHost ...
func ReturnPortFromHost(host string) string {
	_, host, err := net.SplitHostPort(host)
	if err != nil {
		log.Println(err)
		return ""
	}
	return host
}

//ReturnIPFromHost ...
func ReturnIPFromHost(host string) string {
	ip, _, err := net.SplitHostPort(host)
	if err != nil {
		log.Println(err)
		return ""
	}
	return ip
}

//RedirectTLS ...
func RedirectTLS(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "https://"+ReturnPortFromHost(r.Host), http.StatusMovedPermanently)
}

//RandomStringBytes ...
func RandomStringBytes(n int) string {
	const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Int63()%int64(len(letterBytes))]
	}
	return string(b)
}

//AddRemoteAddrToRequest ...
func AddRemoteAddrToRequest(r *http.Request) *http.Request {
	r.Header.Set("X-Forwarded-For", r.RemoteAddr)
	return r
}

//SetCookieToResponse ...
func SetCookieToResponse(w http.ResponseWriter, hash string, configuration *confg.Configuration) http.ResponseWriter {
	http.SetCookie(w, &http.Cookie{Name: "_balansir_server_hash", Value: hash, MaxAge: configuration.SessionMaxAge})
	return w
}

//ServerPoolsEquals ...
func ServerPoolsEquals(serverPoolHash *string, prevPoolHash string, incomingPool []*confg.Endpoint) bool {
	var sumOfServerHash string
	for _, server := range incomingPool {
		serialized, _ := json.Marshal(server)
		sumOfServerHash += string(serialized)
	}
	md := md5.Sum([]byte(sumOfServerHash))
	poolHash := hex.EncodeToString(md[:16])
	if prevPoolHash == poolHash {
		return false
	}
	*serverPoolHash = poolHash
	return true
}

//ServeDistributor ...
func ServeDistributor(endpoint *serverutil.Server, w http.ResponseWriter, r *http.Request, gzipEnabled bool) {
	if gzipEnabled {
		gziputil.ServeWithGzip(endpoint, w, r)
		return
	}
	connection, err := net.DialTimeout("tcp", endpoint.URL.Host, time.Second*2)
	if err != nil {
		return
	}
	connection.Close()
	endpoint.Proxy.ServeHTTP(w, r)
}

//ProxyErrorHandler ...
func ProxyErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		// Suppress `context canceled` error.
		// It may occur when client cancels the request with fast refresh
		// or by closing the connection. This error isn't informative at all and it'll
		// just junk the log around.
		if err.Error() == "context canceled" {
		} else {
			log.Printf(`proxy error: %s`, err.Error())
		}
	}
}

//Max ...
func Max(x int, y int) int {
	if x > 100 {
		return 100
	}
	if x < y {
		return y
	}
	return x
}

//Contains ...
func Contains(path string, prefixes []*confg.Rule) (ok bool, ttl string) {
	for _, rule := range prefixes {
		if strings.HasPrefix(path, rule.Path) {
			return true, rule.TTL
		}
	}
	return false, ""
}

//GetDuration ...
func GetDuration(TTL string) time.Duration {
	if TTL == "" {
		// If TTL isn't specified then return go's max time as Unix int64 value,
		// so in this case the stored response won't be evicted from cache at all.
		// See https://stackoverflow.com/a/25065327
		return 9223372036854775807
	}

	splittedTTL := strings.Split(TTL, ".")
	val, err := strconv.Atoi(splittedTTL[0])

	if err != nil {
		log.Fatal(err)
	}
	unit := splittedTTL[1]

	var duration time.Duration
	switch strings.ToLower(unit) {
	case "second":
		duration = time.Duration(time.Duration(val) * time.Second)
	case "minute":
		duration = time.Duration(time.Duration(val) * time.Minute)
	case "hour":
		duration = time.Duration(time.Duration(val) * time.Hour)
	}

	return duration
}
