package configutil

import (
	"balansir/internal/poolutil"
	"balansir/internal/serverutil"
	"crypto/md5"
	"encoding/hex"
	"expvar"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

//Configuration ...
type Configuration struct {
	Mux                      sync.RWMutex
	ServerList               []*Endpoint `json:"server_list"`
	Protocol                 string      `json:"connection_protocol"`
	SSLCertificate           string      `json:"ssl_certificate"`
	SSLKey                   string      `json:"ssl_private_key"`
	Port                     int         `json:"http_port"`
	TLSPort                  int         `json:"https_port"`
	Delay                    int         `json:"server_check_timer"`
	SessionPersistence       bool        `json:"session_persistence"`
	Autocert                 bool        `json:"autocert"`
	WhiteHosts               []string    `json:"white_hosts"`
	CertDir                  string      `json:"autocert_dir"`
	SessionMaxAge            int         `json:"session_max_age"`
	GzipResponse             bool        `json:"gzip_response"`
	RateLimit                bool        `json:"rate_limit"`
	RatePerSecond            int         `json:"rate_per_second"`
	RateBucket               int         `json:"rate_bucket"`
	Timeout                  int         `json:"server_check_response_timeout"`
	ReadTimeout              int         `json:"read_timeout"`
	WriteTimeout             int         `json:"write_timeout"`
	TransparentProxyMode     bool        `json:"transparent_proxy_mode"`
	Algorithm                string      `json:"balancing_algorithm"`
	Cache                    bool        `json:"cache"`
	CacheShardsAmount        int         `json:"cache_shards_amount"`
	CacheShardMaxSizeMb      int         `json:"cache_shard_max_size_mb"`
	CacheAlgorithm           string      `json:"cache_algorithm"`
	CacheShardExceedFallback bool        `json:"cache_shard_exceed_fallback"`
	CacheBackgroundUpdate    bool        `json:"cache_background_update"`
	CacheRules               []*Rule     `json:"cache_rules"`
}

//Endpoint ...
type Endpoint struct {
	URL    string  `json:"endpoint"`
	Weight float64 `json:"weight"`
}

//Rule ...
type Rule struct {
	Path string `json:"path"`
	TTL  string `json:"ttl"`
}

//RedefineServerPool ...
func RedefineServerPool(configuration *Configuration, serverPoolGuard *sync.WaitGroup, pool *poolutil.ServerPool) {
	var serverHash string
	serverPoolGuard.Add(len(configuration.ServerList))

	pool.ClearPool()

	for index, server := range configuration.ServerList {
		switch configuration.Algorithm {
		case "weighted-round-robin", "weighted-least-connections":
			if server.Weight < 0 {
				log.Fatalf(`Negative weight (%v) is specified for (%s) endpoint in config["server_list"]. Please set it's the weight to 0 if you want to mark it as dead one.`, server.Weight, server.URL)
			} else if server.Weight > 1 {
				log.Fatalf(`Weight can't be greater than 1. You specified (%v) weight for (%s) endpoint in config["server_list"].`, server.Weight, server.URL)
			}
		}

		serverURL, err := url.Parse(configuration.Protocol + "://" + strings.TrimSpace(server.URL))
		if err != nil {
			log.Fatal(err)
		}

		proxy := httputil.NewSingleHostReverseProxy(serverURL)
		proxy.ErrorHandler = proxyErrorHandler
		proxy.Transport = &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   time.Duration(configuration.WriteTimeout) * time.Second,
				KeepAlive: time.Duration(configuration.ReadTimeout) * time.Second,
			}).DialContext,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
		}

		connections := expvar.NewFloat(randomStringBytes(5))

		if configuration.SessionPersistence {
			md := md5.Sum([]byte(serverURL.String()))
			serverHash = hex.EncodeToString(md[:16])
		}

		pool.AddServer(&serverutil.Server{
			URL:               serverURL,
			Weight:            server.Weight,
			ActiveConnections: connections,
			Index:             index,
			Alive:             true,
			Proxy:             proxy,
			ServerHash:        serverHash,
		})
		serverPoolGuard.Done()
	}
}

func proxyErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
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

func randomStringBytes(n int) string {
	const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Int63()%int64(len(letterBytes))]
	}
	return string(b)
}
