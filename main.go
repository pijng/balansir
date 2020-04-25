package main

import (
	"balansir/internal/cacheutil"
	"balansir/internal/confg"
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
	"net/http"
	"net/http/httputil"
	"net/url"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

type tunnel struct {
	wg sync.WaitGroup
}

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
		processingRequests.Done()
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
	processingRequests.Done()
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

	statsChannel := make(chan metricsutil.Stats, 1)
	go startMetricsPolling(statsChannel)
	mp := &metricsutil.MetricsPasser{MetricsChan: statsChannel}
	sm.HandleFunc("/balansir/metrics/stats", mp.MetrictStats)

	sm.Handle("/content/", http.StripPrefix("/content/", http.FileServer(http.Dir("content"))))
	return sm
}

func loadBalance(w http.ResponseWriter, r *http.Request) {
	requestFlow.wg.Wait()
	rateCounter.RateIncrement()
	rtStart := time.Now()
	defer rateCounter.ResponseCount(rtStart)

	processingRequests.Add(1)
	defer processingRequests.Done()

	if configuration.Cache {
		response, err := cacheCluster.Get(r.URL.String())
		if err == nil {
			cacheutil.ServeFromCache(w, r, response)
			return
		}
	}

	availableServers := poolutil.ExcludeUnavailableServers(pool.ServerList)
	if len(availableServers) == 0 {
		// log.Println("all servers are down")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if configuration.RateLimit {
		ip := helpers.ReturnIPFromHost(r.RemoteAddr)
		limiter := visitors.GetVisitor(ip, &visitorMux, &configuration)
		if !limiter.Allow() {
			http.Error(w, http.StatusText(429), http.StatusTooManyRequests)
			return
		}
	}

	if configuration.ProxyMode == "transparent" {
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
			serverPoolWg.Wait()
			for _, server := range pool.ServerList {
				server.CheckAlive(&configuration)
			}
			configuration.Mux.Lock()
			timer = time.NewTicker(time.Duration(configuration.Delay) * time.Second)
			configuration.Mux.Unlock()
		}
	}
}

func startMetricsPolling(stats chan<- metricsutil.Stats) {
	for {
		availableEndpoints := poolutil.ExcludeUnavailableServers(pool.ServerList)
		stats <- metricsutil.GetBalansirStats(rateCounter, &configuration, availableEndpoints)
	}
}

func proxyCacheResponse(r *http.Response) error {
	//Check if URL must be cached
	if ok, TTL := helpers.Contains(r.Request.URL.Path, configuration.CacheRules); ok {

		//Here we're checking if response' url is not cached.
		_, err := cacheCluster.Get(r.Request.URL.Path)
		if err != nil {

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

			//Set complete response to cache
			//`Set` returns an error if response couldn't be written to shard, due to
			//potential exceeding of max capacity.
			//Consider adding some logger here (why?)
			err := cacheCluster.Set(r.Request.URL.Path, respBuf.Bytes(), TTL)
			if err != nil {
				log.Println(err)
			}
		}
	}
	return nil
}

func fillConfiguration(file []byte, config *confg.Configuration) error {
	requestFlow.wg.Add(1)

	processingRequests.Wait()
	defer requestFlow.wg.Done()

	config.Mux.Lock()
	defer config.Mux.Unlock()

	serverPoolWg.Add(1)
	if err := json.Unmarshal(file, &config); err != nil {
		return err
	}
	serverPoolWg.Done()

	if helpers.ServerPoolsEquals(&serverPoolHash, serverPoolHash, configuration.ServerList) {
		var serverHash string
		serverPoolWg.Add(len(configuration.ServerList))

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
			serverPoolWg.Done()
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

		statsChannel := make(chan metricsutil.Stats, 1)
		go startMetricsPolling(statsChannel)
		mp := &metricsutil.MetricsPasser{MetricsChan: statsChannel}
		http.HandleFunc("/balansir/metrics/stats", mp.MetrictStats)

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

var configuration confg.Configuration
var pool poolutil.ServerPool
var serverPoolWg sync.WaitGroup
var requestFlow tunnel
var processingRequests sync.WaitGroup
var serverPoolHash string
var visitors ratelimit.Limiter
var visitorMux sync.Mutex
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

	if configuration.RateLimit {
		go visitors.CleanOldVisitors(&visitorMux)
	}

	if configuration.Cache {
		cacheCluster = cacheutil.New(configuration.CacheShardsAmount, configuration.CacheShardMaxSizeMb, configuration.CacheShardExceedFallback, configuration.CacheAlgorithm)
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
		server := http.Server{
			Addr:         ":" + strconv.Itoa(configuration.Port),
			Handler:      newServeMux(),
			ReadTimeout:  time.Duration(configuration.ReadTimeout) * time.Second,
			WriteTimeout: time.Duration(configuration.WriteTimeout) * time.Second,
		}
		log.Fatal(server.ListenAndServe())
	}

}
