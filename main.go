package main

import (
	"balansir/internal/cacheutil"
	"balansir/internal/configutil"
	"balansir/internal/helpers"
	"balansir/internal/metricsutil"
	"balansir/internal/poolutil"
	"balansir/internal/ratelimit"
	"balansir/internal/rateutil"
	"bytes"
	"crypto/md5"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/acme/autocert"
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

	metricsPolling()
	sm.HandleFunc("/balansir/metrics/stats", metricsutil.MetrictStats)

	sm.Handle("/content/", http.StripPrefix("/content/", http.FileServer(http.Dir("content"))))
	return sm
}

func loadBalance(w http.ResponseWriter, r *http.Request) {
	configurationGuard.Wait()

	if configuration.Cache {
		if ok, _ := helpers.Contains(r.URL.String(), configuration.CacheRules); ok {
			response, err := cacheCluster.Get(r.URL.String(), false)
			if err == nil {
				cacheutil.ServeFromCache(w, r, response)
				return
			}

			hashedKey := cacheCluster.Hash.Sum(r.URL.String())
			guard := cacheCluster.Queue.Get(hashedKey)
			//If there is no queue for a given key – create queue and set release on timeout.
			//Timeout should prevent situation when release won't be triggered in proxyCacheResponse
			//due to server timeouts
			if guard == nil {
				cacheCluster.Queue.Set(hashedKey)
				go func() {
					for {
						select {
						case <-time.After(time.Duration(configuration.WriteTimeout) * time.Second):
							cacheCluster.Queue.Release(hashedKey)
							return
						}
					}
				}()
			} else {
				//If there is a queue for a given key – wait for it to be released and get the response
				//from the cache. Optimistically we don't need to check the returned error in this case,
				//because the only error is a "key not found" yet we immediatelly grab the value after
				//cache set.
				guard.Wait()
				response, _ := cacheCluster.Get(r.URL.String(), false)
				cacheutil.ServeFromCache(w, r, response)
				return
			}
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

	if key := r.Context().Value("background-update"); key == nil {
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
				server.CheckAlive(&configuration.Timeout)
			}
			configuration.Mux.Lock()
			timer = time.NewTicker(time.Duration(configuration.Delay) * time.Second)
			configuration.Mux.Unlock()
		}
	}
}

func metricsPolling() {
	metricsutil.AssignMetricsObjects(rateCounter, &configuration, pool.ServerList, cacheCluster)
}

func proxyCacheResponse(r *http.Response) error {
	//Check if URL must be cached
	if ok, TTL := helpers.Contains(r.Request.URL.Path, configuration.CacheRules); ok {
		trackMiss := r.Request.Context().Value("background-update") != nil

		//Here we're checking if response' url is not cached.
		_, err := cacheCluster.Get(r.Request.URL.Path, trackMiss)
		if err != nil {
			hashedKey := cacheCluster.Hash.Sum(r.Request.URL.Path)
			defer cacheCluster.Queue.Release(hashedKey)

			//Create bytes buffer for headers and iterate over them
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

func fillConfiguration(file []byte, config *configutil.Configuration) []error {
	configurationGuard.Add(1)
	defer configurationGuard.Done()

	config.Mux.Lock()
	defer config.Mux.Unlock()

	serverPoolGuard.Add(1)
	defer serverPoolGuard.Done()

	var errors []error
	if err := json.Unmarshal(file, &config); err != nil {
		errors = append(errors, err)
		return errors
	}

	if !helpers.ServerPoolsEquals(&serverPoolHash, config.ServerList) {
		newPool, err := poolutil.RedefineServerPool(config, &serverPoolGuard)
		if err != nil {
			errors = append(errors, err)
		}
		if newPool != nil {
			pool = newPool
			for _, server := range pool.ServerList {
				server.Proxy.ModifyResponse = proxyCacheResponse
				server.Proxy.ErrorHandler = helpers.ProxyErrorHandler
			}
		}
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

		if !helpers.CacheEquals(&cacheHash, &args) {
			newCacheCluster, err := cacheutil.RedefineCache(&args, cacheCluster)
			if err != nil {
				errors = append(errors, err)
			}
			if newCacheCluster != nil {
				cacheCluster = newCacheCluster
			}
		}
	}

	metricsutil.AssignMetricsObjects(rateCounter, &configuration, pool.ServerList, cacheCluster)
	return errors
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
				log.Println(err)
				continue
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

		metricsPolling()
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
var pool *poolutil.ServerPool
var serverPoolGuard sync.WaitGroup
var configurationGuard sync.WaitGroup
var serverPoolHash string
var cacheHash string
var visitors *ratelimit.Limiter
var cacheCluster *cacheutil.CacheCluster
var rateCounter *rateutil.Rate

func main() {
	file, err := ioutil.ReadFile("config.json")
	if err != nil {
		log.Fatal(err)
	}

	if err := fillConfiguration(file, &configuration); err != nil {
		log.Println(err)
	}

	go serversCheck()
	go configWatch()

	visitors = ratelimit.NewLimiter()

	if configuration.RateLimit {
		go visitors.CleanOldVisitors()
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
