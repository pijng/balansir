package main

import (
	"balansir/internal/balancing"
	"balansir/internal/cacheutil"
	"balansir/internal/configutil"
	"balansir/internal/helpers"
	"balansir/internal/logutil"
	"balansir/internal/metricsutil"
	"balansir/internal/poolutil"
	"balansir/internal/proxyutil"
	"balansir/internal/ratelimit"
	"balansir/internal/rateutil"
	"crypto/md5"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

func newServeMux() *http.ServeMux {
	sm := http.NewServeMux()
	metricsPolling()
	sm.HandleFunc("/", loadBalance)
	sm.HandleFunc("/balansir/metrics", metricsutil.Metrics)
	sm.HandleFunc("/balansir/logs", metricsutil.Metrics)
	sm.HandleFunc("/balansir/logs/collected_logs", metricsutil.CollectedLogs)
	sm.HandleFunc("/balansir/metrics/stats", metricsutil.MetrictStats)
	sm.HandleFunc("/balansir/metrics/collected_stats", metricsutil.CollectedStats)

	sm.Handle("/content/", http.StripPrefix("/content/", http.FileServer(http.Dir("content"))))
	return sm
}

func loadBalance(w http.ResponseWriter, r *http.Request) {
	configurationGuard.Wait()
	configuration := configutil.GetConfig()

	if configuration.Cache {
		if ok, _ := helpers.Contains(r.URL.String(), configuration.CacheRules); ok {
			cache := cacheutil.GetCluster()

			response, err := cache.Get(r.URL.String(), false)
			if err == nil {
				cacheutil.ServeFromCache(w, r, response)
				return
			}

			hashedKey := cache.Hash.Sum(r.URL.String())
			guard := cache.Queue.Get(hashedKey)
			//If there is no queue for a given key – create queue and set release on timeout.
			//Timeout should prevent situation when release won't be tri  ggered in modifyResponse
			//due to server timeouts
			if guard == nil {
				cache.Queue.Set(hashedKey)
				go func() {
					for {
						select {
						case <-time.After(time.Duration(configuration.WriteTimeout) * time.Second):
							cache.Queue.Release(hashedKey)
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
				response, _ := cache.Get(r.URL.String(), false)
				cacheutil.ServeFromCache(w, r, response)
				return
			}
		}
	}

	if configuration.RateLimit {
		ip := helpers.ReturnIPFromHost(r.RemoteAddr)
		limiter := visitors.GetVisitor(ip, configuration)
		if !limiter.Allow() {
			http.Error(w, http.StatusText(429), http.StatusTooManyRequests)
			return
		}
	}

	pool := poolutil.GetPool()
	availableServers := poolutil.ExcludeUnavailableServers(pool.ServerList)
	if len(availableServers) == 0 {
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
				logutil.Warning(err)
				return
			}
			endpoint.Proxy.ServeHTTP(w, r)
			return
		}
	}

	switch configuration.Algorithm {
	case "round-robin":
		balancing.RoundRobin(w, r)

	case "weighted-round-robin":
		balancing.WeightedRoundRobin(w, r)

	case "least-connections":
		balancing.LeastConnections(w, r)

	case "weighted-least-connections":
		balancing.WeightedLeastConnections(w, r)
	}
}

func serversCheck() {
	configuration := configutil.GetConfig()
	pool := poolutil.GetPool()
	timer := time.NewTicker(time.Duration(configuration.Delay) * time.Second)
	for {
		select {
		case <-timer.C:
			serverPoolGuard.Wait()
			inActive := 0
			for _, server := range pool.ServerList {
				active := server.CheckAlive(&configuration.Timeout)
				if !active {
					inActive++
				}
			}
			if inActive == len(pool.ServerList) {
				logutil.Error("All servers are down!")
			}
			configuration.Mux.Lock()
			timer = time.NewTicker(time.Duration(configuration.Delay) * time.Second)
			configuration.Mux.Unlock()
		}
	}
}

func metricsPolling() {
	configuration := configutil.GetConfig()
	pool := poolutil.GetPool()
	metricsutil.InitMetricsMeta(rateCounter, configuration, pool.ServerList)
}

func fillConfiguration(file []byte) []error {
	configurationGuard.Add(1)
	defer configurationGuard.Done()

	configuration := configutil.GetConfig()

	configuration.Mux.Lock()
	defer configuration.Mux.Unlock()

	serverPoolGuard.Add(1)
	defer serverPoolGuard.Done()

	var errs []error
	if err := json.Unmarshal(file, &configuration); err != nil {
		errs = append(errs, errors.New(fmt.Sprint("config.json malformed: ", err)))
		return errs
	}

	pool := poolutil.GetPool()

	if !helpers.ServerPoolsEquals(&serverPoolHash, configuration.ServerList) {
		newPool, err := poolutil.RedefineServerPool(configuration, &serverPoolGuard)
		if err != nil {
			errs = append(errs, err)
		}
		if newPool != nil {
			pool = poolutil.SetPool(newPool)
			for _, server := range pool.ServerList {
				server.Proxy.ModifyResponse = proxyutil.ModifyResponse
				server.Proxy.ErrorHandler = proxyutil.ErrorHandler
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
			err := cacheutil.RedefineCache(&args)
			if err != nil {
				errs = append(errs, err)
			}
		}
	}

	metricsutil.InitMetricsMeta(rateCounter, configuration, pool.ServerList)
	return errs
}

func configWatch() {
	file, err := ioutil.ReadFile("config.json")
	if err != nil {
		logutil.Error(fmt.Sprintf("Error reading configuration file: %v", err))
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
			errs := fillConfiguration(file)
			if errs != nil {
				logutil.Error("Configuration errors:")
				for i := 0; i < len(errs); i++ {
					logutil.Error(fmt.Sprintf("\t %v", errs[i]))
				}
				continue
			}
			logutil.Notice("Configuration changes applied to Balansir")
		}
		time.Sleep(time.Second)
	}
}

func listenAndServeTLSWithAutocert() {
	configuration := configutil.GetConfig()

	certManager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(configuration.AutocertHosts...),
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
		metricsPolling()

		http.HandleFunc("/", loadBalance)
		http.HandleFunc("/balansir/metrics", metricsutil.Metrics)
		http.HandleFunc("/balansir/logs", metricsutil.Metrics)
		http.HandleFunc("/balansir/logs/collected_logs", metricsutil.CollectedLogs)
		http.HandleFunc("/balansir/metrics/stats", metricsutil.MetrictStats)
		http.HandleFunc("/balansir/metrics/collected_stats", metricsutil.CollectedStats)

		err := http.ListenAndServe(
			":"+strconv.Itoa(port),
			certManager.HTTPHandler(nil),
		)
		if err != nil {
			logutil.Fatal(fmt.Sprintf("Error starting listener: %s", err))
			logutil.Fatal("Shutdown")
			os.Exit(1)
		}
	}()

	err := server.ListenAndServeTLS("", "")
	if err != nil {
		logutil.Fatal(fmt.Sprintf("Error starting TLS listener: %s", err))
		logutil.Fatal("Shutdown")
		os.Exit(1)
	} else {
		logutil.Notice("Balansir is up!")
	}
}

func listenAndServeTLSWithSelfSignedCerts() {
	configuration := configutil.GetConfig()
	go func() {
		server := http.Server{
			Addr:         ":" + strconv.Itoa(configuration.Port),
			Handler:      http.HandlerFunc(helpers.RedirectTLS),
			ReadTimeout:  time.Duration(configuration.ReadTimeout) * time.Second,
			WriteTimeout: time.Duration(configuration.WriteTimeout) * time.Second,
		}
		logutil.Fatal(server.ListenAndServe())
	}()

	logutil.Notice("Balansir is up!")
	if err := http.ListenAndServeTLS(":"+strconv.Itoa(configuration.TLSPort), configuration.SSLCertificate, configuration.SSLKey, newServeMux()); err != nil {
		logutil.Fatal(fmt.Sprintf("Error starting TLS listener: %s", err))
		logutil.Fatal("Shutdown")
		os.Exit(1)
	}
}

var serverPoolGuard sync.WaitGroup
var configurationGuard sync.WaitGroup
var serverPoolHash string
var cacheHash string
var visitors *ratelimit.Limiter
var rateCounter *rateutil.Rate

func main() {
	logutil.Init()
	logutil.Info("Booting up...")

	file, err := ioutil.ReadFile("config.json")
	if err != nil {
		logutil.Fatal(fmt.Sprintf("Error reading configuration file: %v", err))
		logutil.Fatal("Shutdown")
		os.Exit(1)
	}

	if errs := fillConfiguration(file); errs != nil {
		logutil.Fatal("Configuration errors:")
		for i := 0; i < len(errs); i++ {
			logutil.Fatal(fmt.Sprintf("\t %v", errs[i]))
			if len(errs)-1 == i {
				logutil.Fatal("Shutdown")
				os.Exit(1)
			}
		}
	}

	go serversCheck()
	go configWatch()

	visitors = ratelimit.NewLimiter()

	configuration := configutil.GetConfig()

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
		logutil.Notice("Balansir is up!")
		logutil.Fatal(server.ListenAndServe())
	}

}
