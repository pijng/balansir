package helpers

import (
	"balansir/internal/cacheutil"
	"balansir/internal/configutil"
	"balansir/internal/logutil"
	"balansir/internal/serverutil"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
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
		logutil.Warning(err)
		return ""
	}
	return ip
}

//RedirectTLS ...
func RedirectTLS(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "https://"+ReturnPortFromHost(r.Host), http.StatusMovedPermanently)
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

//SetCookieToResponse ...
func SetCookieToResponse(w http.ResponseWriter, hash string, configuration *configutil.Configuration) http.ResponseWriter {
	http.SetCookie(w, &http.Cookie{Name: "_balansir_server_hash", Value: hash, MaxAge: configuration.SessionMaxAge})
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

//CacheEquals ...
func CacheEquals(cacheHash *string, incomingArgs *cacheutil.CacheClusterArgs) bool {
	serialized, _ := json.Marshal(incomingArgs)
	md := md5.Sum(serialized)
	newCacheHash := hex.EncodeToString(md[:16])
	if *cacheHash == newCacheHash {
		return true
	}
	*cacheHash = newCacheHash
	return false
}

//ServeDistributor ...
func ServeDistributor(endpoint *serverutil.Server, timeout int, w http.ResponseWriter, r *http.Request, gzipEnabled bool) {
	connection, err := net.DialTimeout("tcp", endpoint.URL.Host, time.Second*time.Duration(timeout))
	if err != nil {
		return
	}
	connection.Close()

	w = setSecureHeaders(w)
	endpoint.Proxy.ServeHTTP(w, r)
}

//Contains ...
func Contains(path string, prefixes []*configutil.Rule) (ok bool, ttl string) {
	for _, rule := range prefixes {
		if strings.HasPrefix(path, rule.Path) {
			return true, rule.TTL
		}
	}
	return false, ""
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

type delayed func(interface{})

type dMeta struct {
	dC int64
}

var limiter *dMeta

//CallLimit ...
func CallLimit(tick time.Duration, fn delayed, arg string) {
	if limiter == nil {
		limiter = &dMeta{
			dC: 0,
		}
		go func() {
			timer := time.NewTicker(time.Second * tick)
			for {
				select {
				case <-timer.C:
					atomic.StoreInt64(&limiter.dC, 0)
				}
			}
		}()
	}

	if atomic.LoadInt64(&limiter.dC) == 0 {
		fn(arg)
		atomic.AddInt64(&limiter.dC, 1)
	}
}
