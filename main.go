package main

import (
	"balansir/internal/confg"
	"balansir/internal/helpers"
	"balansir/internal/poolutil"
	"balansir/internal/serverutil"
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
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

type tunnel struct {
	mux sync.RWMutex
	wg  sync.WaitGroup
}

func loadBalance(w http.ResponseWriter, r *http.Request) {
	requestFlow.mux.Lock()
	requestFlow.wg.Wait()
	requestFlow.mux.Unlock()
	if configuration.ProxyMode == "transparent" {
		r = helpers.AddRemoteAddrToRequest(r)
	}

	if configuration.SessionPersistence {
		cookieHash, _ := r.Cookie("_balansir_server_hash")
		if cookieHash != nil {
			endpoint, err := pool.GetServerByHash(cookieHash.Value)
			if err == nil {
				endpoint.Proxy.ServeHTTP(w, r)
				return
			}
		}
	}

	switch configuration.Algorithm {
	case "round-robin":
		processingRequests.Add(1)
		index := pool.NextPool()
		endpoint := pool.ServerList[index]
		if configuration.SessionPersistence {
			w = helpers.SetCookieToResponse(w, endpoint.ServerHash, &configuration)
		}
		endpoint.Proxy.ServeHTTP(w, r)
		processingRequests.Done()

	case "weighted-round-robin":
		processingRequests.Add(1)
		poolChoice := pool.GetPoolChoice()
		endpoint, err := poolutil.WeightedChoice(poolChoice)
		if err != nil {
			log.Println(err)
		}
		if configuration.SessionPersistence {
			w = helpers.SetCookieToResponse(w, endpoint.ServerHash, &configuration)
		}
		endpoint.Proxy.ServeHTTP(w, r)
		processingRequests.Done()

	case "least-connections":
		processingRequests.Add(1)
		endpoint := pool.GetLeastConnectedServer()
		if configuration.SessionPersistence {
			w = helpers.SetCookieToResponse(w, endpoint.ServerHash, &configuration)
		}
		endpoint.ActiveConnections.Add(1)
		endpoint.Proxy.ServeHTTP(w, r)
		endpoint.ActiveConnections.Add(-1)
		processingRequests.Done()

	case "weighted-least-connections":
		processingRequests.Add(1)
		endpoint := pool.GetWeightedLeastConnectedServer()
		if configuration.SessionPersistence {
			w = helpers.SetCookieToResponse(w, endpoint.ServerHash, &configuration)
		}
		endpoint.ActiveConnections.Add(1)
		endpoint.Proxy.ServeHTTP(w, r)
		endpoint.ActiveConnections.Add(-1)
		processingRequests.Done()
	}
}

func serversCheck() {
	timer := time.NewTicker(time.Duration(configuration.Delay) * time.Second)
	for {
		select {
		case <-timer.C:
			wg.Wait()
			for _, server := range pool.ServerList {
				server.CheckAlive(&configuration)
			}
			configuration.Mux.Lock()
			timer = time.NewTicker(time.Duration(configuration.Delay) * time.Second)
			configuration.Mux.Unlock()
		}
	}
}

func fillConfiguration(file []byte, config *confg.Configuration) {
	requestFlow.mux.Lock()
	requestFlow.wg.Add(1)
	requestFlow.mux.Unlock()

	processingRequests.Wait()
	config.Mux.Lock()

	wg.Add(1)
	json.Unmarshal(file, &config)
	wg.Done()

	if helpers.ServerPoolsEquals(&serverPoolHash, serverPoolHash, configuration.ServerList) {
		var serverHash string
		wg.Add(len(configuration.ServerList))

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
			wg.Done()
		}

		switch configuration.Algorithm {
		case "weighted-round-robin", "weighted-least-connections":
			nonZeroServers := pool.ExcludeZeroWeightServers()
			if len(nonZeroServers) <= 0 {
				log.Fatalf(`0 weight is specified for all your endpoints in config["server_list"]. Please consider adding at least one endpoint with non-zero weight.`)
			}
		}
	}
	config.Mux.Unlock()
	requestFlow.wg.Done()
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
			fillConfiguration(file, &configuration)
			log.Println("Configuration file changes applied to Balansir")
		}
		time.Sleep(time.Second)
	}
}

func listenAndServeTLSWithAutocert() {

	certManager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(configuration.WhiteHosts),
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

	go func() {
		http.HandleFunc("/", loadBalance)
		http.ListenAndServe(
			":"+strconv.Itoa(configuration.Port),
			certManager.HTTPHandler(nil),
		)
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

	if err := http.ListenAndServeTLS(":"+strconv.Itoa(configuration.TLSPort), configuration.SSLCertificate, configuration.SSLKey, http.HandlerFunc(loadBalance)); err != nil {
		log.Fatalf(`Error starting TLS listener: %s`, err)
	}
}

var configuration confg.Configuration
var pool poolutil.ServerPool
var wg sync.WaitGroup
var requestFlow tunnel
var processingRequests sync.WaitGroup
var serverPoolHash string

func main() {
	file, err := ioutil.ReadFile("config.json")
	if err != nil {
		log.Fatal(err)
	}

	fillConfiguration(file, &configuration)

	go serversCheck()
	go configWatch()

	if configuration.Protocol == "https" {

		if configuration.Autocert {
			listenAndServeTLSWithAutocert()
		} else {
			listenAndServeTLSWithSelfSignedCerts()
		}
	} else {
		server := http.Server{
			Addr:         ":" + strconv.Itoa(configuration.Port),
			Handler:      http.HandlerFunc(loadBalance),
			ReadTimeout:  time.Duration(configuration.ReadTimeout) * time.Second,
			WriteTimeout: time.Duration(configuration.WriteTimeout) * time.Second,
		}
		log.Fatal(server.ListenAndServe())
	}

}
