package main

import (
	"balansir/internal/balanceutil"
	"balansir/internal/cacheutil"
	"balansir/internal/configutil"
	"balansir/internal/helpers"
	"balansir/internal/limitutil"
	"balansir/internal/logutil"
	"balansir/internal/metricsutil"
	"balansir/internal/poolutil"
	"balansir/internal/rateutil"
	"balansir/internal/staticutil"
	"crypto/md5"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/acme/autocert"
	"gopkg.in/yaml.v2"
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

	if configuration.ServeStatic {
		if staticutil.IsStatic(r.URL.Path) {
			err := staticutil.TryServeStatic(w, r)
			if err == nil {
				return
			}
			logutil.Warning(err)
		}
	}

	if configuration.Cache {
		if err := cacheutil.TryServeFromCache(w, r); err == nil {
			return
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

	if configuration.TransparentProxy {
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
	case balanceutil.RoundRobinType:
		balanceutil.RoundRobin(w, r)

	case balanceutil.WeightedRoundRobinType:
		balanceutil.WeightedRoundRobin(w, r)

	case balanceutil.LeastConnectionsType:
		balanceutil.LeastConnections(w, r)

	case balanceutil.WeightedLeastConnectionsType:
		balanceutil.WeightedLeastConnections(w, r)
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
	if err := yaml.Unmarshal(file, &configuration); err != nil {
		errs = append(errs, errors.New(fmt.Sprint("config.yml malformed: ", err)))
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
		}
	}

	if configuration.Cache {
		args := cacheutil.CacheClusterArgs{
			ShardsAmount:     configuration.CacheShardsAmount,
			MaxSize:          configuration.CacheShardSize,
			ExceedFallback:   configuration.CacheShardExceedFallback,
			CacheAlgorithm:   configuration.CacheAlgorithm,
			BackgroundUpdate: configuration.CacheBackgroundUpdate,
			TransportTimeout: configuration.WriteTimeout,
			DialerTimeout:    configuration.ReadTimeout,
			CacheRules:       configuration.CacheRules,
			Port:             configuration.Port,
		}

		if !cacheutil.CacheEquals(&cacheHash, &args) {
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
	file, err := ioutil.ReadFile("config.yml")
	if err != nil {
		logutil.Error(fmt.Sprintf("Error reading configuration file: %v", err))
	}
	md := md5.Sum(file)
	fileHash := hex.EncodeToString(md[:16])
	var fileHashNext string
	for {
		file, _ = ioutil.ReadFile("config.yml")
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
var visitors *limitutil.Limiter
var rateCounter *rateutil.Rate

func main() {
	logutil.Init()
	logutil.Info("Booting up...")

	file, err := ioutil.ReadFile("config.yml")
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

	visitors = limitutil.NewLimiter()

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
