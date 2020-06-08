package main

import (
	"balansir/internal/cacheutil"
	"balansir/internal/configutil"
	"balansir/internal/metricsutil"
	"balansir/internal/poolutil"
	"balansir/internal/ratelimit"
	"balansir/internal/rateutil"
	"crypto/md5"
	"crypto/tls"
	"encoding/hex"
	"io/ioutil"
	"log"
	"net/http"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/acme/autocert"
	// _ "net/http/pprof"
)

func roundRobin(w http.ResponseWriter, r *http.Request) {
	index := pool.NextPool()
	endpoint := pool.ServerList[index]
	if configuration.SessionPersistence {
		w = setCookieToResponse(w, endpoint.ServerHash, &configuration)
	}
	serveDistributor(endpoint, configuration.Timeout, w, r, configuration.GzipResponse)
}

func weightedRoundRobin(w http.ResponseWriter, r *http.Request) {
	poolChoice := pool.GetPoolChoice()
	endpoint, err := poolutil.WeightedChoice(poolChoice)
	if err != nil {
		processingRequests.Done()
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if configuration.SessionPersistence {
		w = setCookieToResponse(w, endpoint.ServerHash, &configuration)
	}
	serveDistributor(endpoint, configuration.Timeout, w, r, configuration.GzipResponse)
}

func leastConnections(w http.ResponseWriter, r *http.Request) {
	endpoint := pool.GetLeastConnectedServer()
	if configuration.SessionPersistence {
		w = setCookieToResponse(w, endpoint.ServerHash, &configuration)
	}
	endpoint.ActiveConnections.Add(1)
	serveDistributor(endpoint, configuration.Timeout, w, r, configuration.GzipResponse)
	endpoint.ActiveConnections.Add(-1)
	processingRequests.Done()
}

func weightedLeastConnections(w http.ResponseWriter, r *http.Request) {
	endpoint := pool.GetWeightedLeastConnectedServer()
	if configuration.SessionPersistence {
		w = setCookieToResponse(w, endpoint.ServerHash, &configuration)
	}
	endpoint.ActiveConnections.Add(1)
	serveDistributor(endpoint, configuration.Timeout, w, r, configuration.GzipResponse)
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
	requestFlow.Wait()

	processingRequests.Add(1)
	defer processingRequests.Done()

	if configuration.Cache {
		if ok, _ := configutil.Contains(r.URL.String(), configuration.CacheRules); ok {
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
		ip := returnIPFromHost(r.RemoteAddr)
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
		r = addRemoteAddrToRequest(r)
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
			serverPoolWg.Wait()
			for _, server := range pool.ServerList {
				server.CheckAlive(configuration.Timeout)
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
			args := configutil.FillConfigurationArgs{
				File:               file,
				Config:             &configuration,
				Cluster:            cacheCluster,
				RequestFlow:        &requestFlow,
				ProcessingRequests: &processingRequests,
				ServerPoolWg:       &serverPoolWg,
				ServerPoolHash:     &serverPoolHash,
			}
			err := configutil.FillConfiguration(args)
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
			Handler:      http.HandlerFunc(redirectTLS),
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
var serverPoolWg sync.WaitGroup
var requestFlow sync.WaitGroup
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

	args := configutil.FillConfigurationArgs{
		File:               file,
		Config:             &configuration,
		Cluster:            cacheCluster,
		RequestFlow:        &requestFlow,
		ProcessingRequests: &processingRequests,
		ServerPoolWg:       &serverPoolWg,
		ServerPoolHash:     &serverPoolHash,
	}
	if err := configutil.FillConfiguration(args); err != nil {
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
