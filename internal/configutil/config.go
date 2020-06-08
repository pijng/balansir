package configutil

import (
	"balansir/internal/cacheutil"
	"balansir/internal/poolutil"
	"balansir/internal/serverutil"
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"expvar"
	"io/ioutil"
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
	ServerList               []*Endpoint       `json:"server_list"`
	Protocol                 string            `json:"connection_protocol"`
	SSLCertificate           string            `json:"ssl_certificate"`
	SSLKey                   string            `json:"ssl_private_key"`
	Port                     int               `json:"http_port"`
	TLSPort                  int               `json:"https_port"`
	Delay                    int               `json:"server_check_timer"`
	SessionPersistence       bool              `json:"session_persistence"`
	Autocert                 bool              `json:"autocert"`
	WhiteHosts               []string          `json:"white_hosts"`
	CertDir                  string            `json:"autocert_dir"`
	SessionMaxAge            int               `json:"session_max_age"`
	GzipResponse             bool              `json:"gzip_response"`
	RateLimit                bool              `json:"rate_limit"`
	RatePerSecond            int               `json:"rate_per_second"`
	RateBucket               int               `json:"rate_bucket"`
	Timeout                  int               `json:"server_check_response_timeout"`
	ReadTimeout              int               `json:"read_timeout"`
	WriteTimeout             int               `json:"write_timeout"`
	TransparentProxyMode     bool              `json:"transparent_proxy_mode"`
	Algorithm                string            `json:"balancing_algorithm"`
	Cache                    bool              `json:"cache"`
	CacheShardsAmount        int               `json:"cache_shards_amount"`
	CacheShardMaxSizeMb      int               `json:"cache_shard_max_size_mb"`
	CacheAlgorithm           string            `json:"cache_algorithm"`
	CacheShardExceedFallback bool              `json:"cache_shard_exceed_fallback"`
	CacheBackgroundUpdate    bool              `json:"cache_background_update"`
	CacheRules               []*cacheutil.Rule `json:"cache_rules"`
}

//Endpoint ...
type Endpoint struct {
	URL    string  `json:"endpoint"`
	Weight float64 `json:"weight"`
}

var configuration *Configuration
var cacheCluster *cacheutil.CacheCluster

//FillConfigurationArgs ...
type FillConfigurationArgs struct {
	File               []byte
	Config             *Configuration
	Cluster            *cacheutil.CacheCluster
	RequestFlow        *sync.WaitGroup
	ProcessingRequests *sync.WaitGroup
	ServerPoolWg       *sync.WaitGroup
	ServerPoolHash     *string
	Pool               *poolutil.ServerPool
}

//FillConfiguration ...
func FillConfiguration(args FillConfigurationArgs) error {
	args.RequestFlow.Add(1)
	defer args.RequestFlow.Done()

	args.ProcessingRequests.Wait()

	args.Config.Mux.Lock()
	defer args.Config.Mux.Unlock()

	args.ServerPoolWg.Add(1)
	if err := json.Unmarshal(args.File, &args.Config); err != nil {
		return err
	}
	args.ServerPoolWg.Done()

	configuration = args.Config
	cacheCluster = args.Cluster

	if serverPoolsEquals(args.ServerPoolHash, args.Config.ServerList) {
		var serverHash string
		args.ServerPoolWg.Add(len(args.Config.ServerList))

		args.Pool.ClearPool()

		for index, server := range args.Config.ServerList {
			switch args.Config.Algorithm {
			case "weighted-round-robin", "weighted-least-connections":
				if server.Weight < 0 {
					log.Fatalf(`Negative weight (%v) is specified for (%s) endpoint in config["server_list"]. Please set it's the weight to 0 if you want to mark it as dead one.`, server.Weight, server.URL)
				} else if server.Weight > 1 {
					log.Fatalf(`Weight can't be greater than 1. You specified (%v) weight for (%s) endpoint in config["server_list"].`, server.Weight, server.URL)
				}
			}

			serverURL, err := url.Parse(args.Config.Protocol + "://" + strings.TrimSpace(server.URL))
			if err != nil {
				log.Fatal(err)
			}

			proxy := httputil.NewSingleHostReverseProxy(serverURL)
			proxy.ErrorHandler = proxyErrorHandler
			proxy.Transport = &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   time.Duration(args.Config.WriteTimeout) * time.Second,
					KeepAlive: time.Duration(args.Config.ReadTimeout) * time.Second,
				}).DialContext,
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
			}

			if args.Config.Cache {
				proxy.ModifyResponse = proxyCacheResponse
			}

			connections := expvar.NewFloat(randomStringBytes(5))

			if args.Config.SessionPersistence {
				md := md5.Sum([]byte(serverURL.String()))
				serverHash = hex.EncodeToString(md[:16])
			}

			args.Pool.AddServer(&serverutil.Server{
				URL:               serverURL,
				Weight:            server.Weight,
				ActiveConnections: connections,
				Index:             index,
				Alive:             true,
				Proxy:             proxy,
				ServerHash:        serverHash,
			})
			args.ServerPoolWg.Done()
		}

		switch args.Config.Algorithm {
		case "weighted-round-robin", "weighted-least-connections":
			nonZeroServers := args.Pool.ExcludeZeroWeightServers()
			if len(nonZeroServers) <= 0 {
				log.Fatalf(`0 weight is specified for all your endpoints in config["server_list"]. Please consider adding at least one endpoint with non-zero weight.`)
			}
		}
	}

	return nil
}

func proxyCacheResponse(r *http.Response) error {
	//Check if URL must be cached
	if ok, TTL := Contains(r.Request.URL.Path, configuration.CacheRules); ok {
		trackMiss := r.Request.Header.Get("X-Balansir-Background-Update") == ""

		//Here we're checking if response' url is not cached.
		_, err := cacheCluster.Get(r.Request.URL.Path, trackMiss)
		if err != nil {
			hashedKey := cacheCluster.Hash.Sum(r.Request.URL.Path)
			defer cacheCluster.Queue.Release(hashedKey)

			//Create byte buffer for all response' headers and iterate over 'em
			headerBuf := bytes.NewBuffer([]byte{})

			for key, val := range r.Header {
				//Write header's key to buffer
				headerBuf.Write([]byte(key))
				//Add delimeter so we can split header key later
				headerBuf.Write([]byte(";-;"))
				//Create byte buffer for header value
				headerValueBuf := bytes.NewBuffer([]byte{})
				//Header value is a string slice, so iterate over it to correctly write it to a buffer
				for _, v := range val {
					headerValueBuf.Write([]byte(v))
				}
				//Write complete header value to headers buffer
				headerBuf.Write(headerValueBuf.Bytes())
				//Add another delimeter so we can split headers out of each other
				headerBuf.Write([]byte(";--;"))
			}

			//Read response body, write it to buffer
			b, _ := ioutil.ReadAll(r.Body)
			bodyBuf := bytes.NewBuffer(b)

			//Reassign response body
			r.Body = ioutil.NopCloser(bodyBuf)

			//Create new buffer. Write our headers and body
			respBuf := bytes.NewBuffer([]byte{})
			respBuf.Write(headerBuf.Bytes())
			respBuf.Write(bodyBuf.Bytes())

			err := cacheCluster.Set(r.Request.URL.Path, respBuf.Bytes(), TTL)
			if err != nil {
				log.Println(err)
			}
		}
	}
	return nil
}

//Contains ...
func Contains(path string, prefixes []*cacheutil.Rule) (ok bool, ttl string) {
	for _, rule := range prefixes {
		if strings.HasPrefix(path, rule.Path) {
			return true, rule.TTL
		}
	}
	return false, ""
}

func serverPoolsEquals(serverPoolHash *string, incomingPool []*Endpoint) bool {
	var sumOfServerHash string
	for _, server := range incomingPool {
		serialized, _ := json.Marshal(server)
		sumOfServerHash += string(serialized)
	}
	md := md5.Sum([]byte(sumOfServerHash))
	poolHash := hex.EncodeToString(md[:16])
	if serverPoolHash == &poolHash {
		return true
	}
	serverPoolHash = &poolHash
	return false
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
