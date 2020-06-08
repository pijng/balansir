package main

import (
	"balansir/internal/cacheutil"
	"balansir/internal/configutil"
	"balansir/internal/helpers"
	"balansir/internal/metricsutil"
	"balansir/internal/poolutil"
	"balansir/internal/ratelimit"
	"balansir/internal/rateutil"
	"balansir/internal/serverutil"
	"bytes"
	"crypto/md5"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"expvar"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/acme/autocert"
	// _ "net/http/pprof"
)

func roundRobin(w http.ResponseWriter, r *http.Request) {
	index := pool.NextPool()
	endpoint := pool.ServerList[index]
	if configuration.SessionPersistence {
		w = helpers.SetCookieToResponse(w, endpoint.ServerHash, &configuration)
	}
	helpers.ServeDistributor(endpoint, configuration.Timeout, w, r, configuration.GzipResponse)
}

func weightedRoundRobin(w http.ResponseWriter, r *http.Request) {
	poolChoice := pool.GetPoolChoice()
	endpoint, err := poolutil.WeightedChoice(poolChoice)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if configuration.SessionPersistence {
		w = helpers.SetCookieToResponse(w, endpoint.ServerHash, &configuration)
	}
	helpers.ServeDistributor(endpoint, configuration.Timeout, w, r, configuration.GzipResponse)
}

func leastConnections(w http.ResponseWriter, r *http.Request) {
	endpoint := pool.GetLeastConnectedServer()
	if configuration.SessionPersistence {
		w = helpers.SetCookieToResponse(w, endpoint.ServerHash, &configuration)
	}
	endpoint.ActiveConnections.Add(1)
	helpers.ServeDistributor(endpoint, configuration.Timeout, w, r, configuration.GzipResponse)
	endpoint.ActiveConnections.Add(-1)
}

func weightedLeastConnections(w http.ResponseWriter, r *http.Request) {
	endpoint := pool.GetWeightedLeastConnectedServer()
	if configuration.SessionPersistence {
		w = helpers.SetCookieToResponse(w, endpoint.ServerHash, &configuration)
	}
	endpoint.ActiveConnections.Add(1)
	helpers.ServeDistributor(endpoint, configuration.Timeout, w, r, configuration.GzipResponse)
	endpoint.ActiveConnections.Add(-1)
}

func newServeMux() *http.ServeMux {
	sm := http.NewServeMux()
	sm.HandleFunc("/", loadBalance)
	sm.HandleFunc("/balansir/metrics", metricsutil.Metrics)

	startMetricsPolling()
	sm.HandleFunc("/balansir/metrics/stats", metricsutil.MetrictStats)

	sm.Handle("/content/", http.StripPrefix("/content/", http.FileServer(http.Dir("content"))))
	return sm
}

func loadBalance(w http.ResponseWriter, r *http.Request) {
	configurationGuard.Wait()

	processingRequests.Add(1)
	defer processingRequests.Done()

	if configuration.Cache {
		if ok, _ := helpers.Contains(r.URL.String(), configuration.CacheRules); ok {
			response, err := cacheCluster.Get(r.URL.String(), false)
			if err == nil {
				cacheutil.ServeFromCache(w, r, response)
				return
			}

			hashedKey := cacheCluster.Hash.Sum(r.URL.String())
			guard := cacheCluster.Queue.Get(hashedKey)
			if guard != nil {
				// Should we add some sort of timeout here? We ensure to release queue on defer in
				// 'proxyCacheResponse' but I just wonder if there are some edge cases when queue
				// won't be released.
				guard.Wait()
				response, _ := cacheCluster.Get(r.URL.String(), false)
				cacheutil.ServeFromCache(w, r, response)
				return
			}
			cacheCluster.Queue.Set(hashedKey)
		}
	}

	if configuration.RateLimit {
		ip := helpers.ReturnIPFromHost(r.RemoteAddr)
		limiter := visitors.GetVisitor(ip, &configuration)
		if !limiter.Allow() {
			http.Error(w, http.StatusText(429), http.StatusTooManyRequests)
			return
		}
	}

	availableServers := poolutil.ExcludeUnavailableServers(pool.ServerList)
	if len(availableServers) == 0 {
		// log.Println("all servers are down")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if r.Header.Get("X-Balansir-Background-Update") == "" {
		rateCounter.RateIncrement()
		rtStart := time.Now()
		defer rateCounter.ResponseCount(rtStart)
	}

	if configuration.TransparentProxyMode {
		r = helpers.AddRemoteAddrToRequest(r)
	}

	if configuration.SessionPersistence {
		cookieHash, _ := r.Cookie("_balansir_server_hash")
		if cookieHash != nil {
			endpoint, err := pool.GetServerByHash(cookieHash.Value)
			if err != nil {
				log.Println(err)
				return
			}
			endpoint.Proxy.ServeHTTP(w, r)
			return
		}
	}

	switch configuration.Algorithm {
	case "round-robin":
		roundRobin(w, r)

	case "weighted-round-robin":
		weightedRoundRobin(w, r)

	case "least-connections":
		leastConnections(w, r)

	case "weighted-least-connections":
		weightedLeastConnections(w, r)
	}
}

func serversCheck() {
	timer := time.NewTicker(time.Duration(configuration.Delay) * time.Second)
	for {
		select {
		case <-timer.C:
			serverPoolGuard.Wait()
			for _, server := range pool.ServerList {
				server.CheckAlive(&configuration)
			}
			configuration.Mux.Lock()
			timer = time.NewTicker(time.Duration(configuration.Delay) * time.Second)
			configuration.Mux.Unlock()
		}
	}
}

func startMetricsPolling() {
	metricsutil.Init(rateCounter, &configuration, pool.ServerList, cacheCluster)
}

func proxyCacheResponse(r *http.Response) error {
	//Check if URL must be cached
	if ok, TTL := helpers.Contains(r.Request.URL.Path, configuration.CacheRules); ok {
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

func fillConfiguration(file []byte, config *configutil.Configuration) error {
	configurationGuard.Add(1)
	defer configurationGuard.Done()

	processingRequests.Wait()

	config.Mux.Lock()
	defer config.Mux.Unlock()

	serverPoolGuard.Add(1)
	defer serverPoolGuard.Done()
	if err := json.Unmarshal(file, &config); err != nil {
		return err
	}

	if helpers.ServerPoolsEquals(&serverPoolHash, serverPoolHash, configuration.ServerList) {
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
			proxy.ErrorHandler = helpers.ProxyErrorHandler
			proxy.Transport = &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   time.Duration(configuration.WriteTimeout) * time.Second,
					KeepAlive: time.Duration(configuration.ReadTimeout) * time.Second,
				}).DialContext,
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
			}

			if configuration.Cache {
				proxy.ModifyResponse = proxyCacheResponse
			}

			connections := expvar.NewFloat(helpers.RandomStringBytes(5))

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

		switch configuration.Algorithm {
		case "weighted-round-robin", "weighted-least-connections":
			nonZeroServers := pool.ExcludeZeroWeightServers()
			if len(nonZeroServers) <= 0 {
				log.Fatalf(`0 weight is specified for all your endpoints in config["server_list"]. Please consider adding at least one endpoint with non-zero weight.`)
			}
		}
	}

	return nil
}

func configWatch() {
	file, err := ioutil.ReadFile("config.json")
	if err != nil {
		log.Fatal(err)
	}
	md := md5.Sum(file)
	fileHash := hex.EncodeToString(md[:16])
	var fileHashNext string
	for {
		file, _ = ioutil.ReadFile("config.json")
		md = md5.Sum(file)
		fileHashNext = hex.EncodeToString(md[:16])
		if fileHash != fileHashNext {
			fileHash = fileHashNext
			err := fillConfiguration(file, &configuration)
			if err != nil {
				log.Fatalf(`Error reading configuration: %s`, err)
			}
			log.Println("Configuration file changes applied to Balansir")
		}
		time.Sleep(time.Second)
	}
}

func listenAndServeTLSWithAutocert() {

	certManager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(configuration.WhiteHosts...),
		Cache:      autocert.DirCache(configuration.CertDir),
	}

	server := &http.Server{
		Addr: ":" + strconv.Itoa(configuration.TLSPort),
		TLSConfig: &tls.Config{
			GetCertificate: certManager.GetCertificate,
		},
		ReadTimeout:  time.Duration(configuration.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(configuration.WriteTimeout) * time.Second,
	}

	port := configuration.Port

	go func() {
		http.HandleFunc("/", loadBalance)
		http.HandleFunc("/balansir/metrics", metricsutil.Metrics)

		startMetricsPolling()
		http.HandleFunc("/balansir/metrics/stats", metricsutil.MetrictStats)

		err := http.ListenAndServe(
			":"+strconv.Itoa(port),
			certManager.HTTPHandler(nil),
		)
		if err != nil {
			log.Fatalf(`Error starting listener: %s`, err)
		}
	}()

	err := server.ListenAndServeTLS("", "")
	if err != nil {
		log.Fatalf(`Error starting TLS listener: %s`, err)
	}
}

func listenAndServeTLSWithSelfSignedCerts() {
	go func() {
		server := http.Server{
			Addr:         ":" + strconv.Itoa(configuration.Port),
			Handler:      http.HandlerFunc(helpers.RedirectTLS),
			ReadTimeout:  time.Duration(configuration.ReadTimeout) * time.Second,
			WriteTimeout: time.Duration(configuration.WriteTimeout) * time.Second,
		}
		log.Fatal(server.ListenAndServe())
	}()

	if err := http.ListenAndServeTLS(":"+strconv.Itoa(configuration.TLSPort), configuration.SSLCertificate, configuration.SSLKey, newServeMux()); err != nil {
		log.Fatalf(`Error starting TLS listener: %s`, err)
	}
}

var configuration configutil.Configuration
var pool poolutil.ServerPool
var serverPoolGuard sync.WaitGroup
var configurationGuard sync.WaitGroup
var processingRequests sync.WaitGroup
var serverPoolHash string
var visitors *ratelimit.Limiter
var cacheCluster *cacheutil.CacheCluster
var rateCounter *rateutil.Rate

func main() {
	file, err := ioutil.ReadFile("config.json")
	if err != nil {
		log.Fatal(err)
	}

	if err := fillConfiguration(file, &configuration); err != nil {
		log.Fatalf(`Error reading configuration: %s`, err)
	}

	go serversCheck()
	go configWatch()

	visitors = ratelimit.NewLimiter()

	if configuration.RateLimit {
		go visitors.CleanOldVisitors()
	}

	if configuration.Cache {
		args := cacheutil.CacheClusterArgs{
			ShardsAmount:     configuration.CacheShardsAmount,
			MaxSize:          configuration.CacheShardMaxSizeMb,
			ExceedFallback:   configuration.CacheShardExceedFallback,
			CacheAlgorithm:   configuration.CacheAlgorithm,
			BackgroundUpdate: configuration.CacheBackgroundUpdate,
			TransportTimeout: configuration.WriteTimeout,
			DialerTimeout:    configuration.ReadTimeout,
			CacheRules:       configuration.CacheRules,
			Port:             configuration.Port,
		}

		cacheCluster = cacheutil.New(args)
		debug.SetGCPercent(cacheutil.GCPercentRatio(configuration.CacheShardsAmount, configuration.CacheShardMaxSizeMb))
		log.Print("Cache enabled")
	}

	rateCounter = rateutil.NewRateCounter()

	if configuration.Protocol == "https" {

		if configuration.Autocert {
			listenAndServeTLSWithAutocert()
		} else {
			listenAndServeTLSWithSelfSignedCerts()
		}
	} else {
		// go func() {
		// 	log.Println(http.ListenAndServe("localhost:8080", nil))
		// }()
		server := http.Server{
			Addr:         ":" + strconv.Itoa(configuration.Port),
			Handler:      newServeMux(),
			ReadTimeout:  time.Duration(configuration.ReadTimeout) * time.Second,
			WriteTimeout: time.Duration(configuration.WriteTimeout) * time.Second,
		}
		log.Fatal(server.ListenAndServe())
	}

}
