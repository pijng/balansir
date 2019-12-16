package main

import (
	"balansir/internal/confg"
	"balansir/internal/helpers"
	"balansir/internal/serverutil"
	"crypto/md5"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"expvar"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

type tunnel struct {
	mux sync.RWMutex
	wg  sync.WaitGroup
}

type endpointChoice struct {
	Endpoint *serverutil.Server
	Weight   int
}

type serverPool struct {
	mux        sync.RWMutex
	ServerList []*serverutil.Server
	Current    int
}

func (pool *serverPool) getPoolChoice() []endpointChoice {
	choice := []endpointChoice{}
	serverList := pool.excludeZeroWeightServers()
	serverList = excludeUnavailableServers(serverList)
	for _, server := range serverList {
		if server.GetAlive() {
			weight := int(server.Weight * 100)
			choice = append(choice, endpointChoice{Weight: weight, Endpoint: server})
		}
	}

	return choice
}

func weightedChoice(choices []endpointChoice) (*serverutil.Server, error) {
	rand.Seed(time.Now().UnixNano())
	weightShum := 0
	for _, choice := range choices {
		weightShum += choice.Weight
	}
	randint := rand.Intn(weightShum)

	sort.Slice(choices, func(i, j int) bool {
		if choices[i].Weight > choices[j].Weight {
			return true
		}
		return false
	})

	for _, choice := range choices {
		if choice.Weight == 100 {
			return choice.Endpoint, nil
		}
		randint -= choice.Weight
		if randint <= 0 {
			return choice.Endpoint, nil
		}
	}
	return &serverutil.Server{}, errors.New("no server returned from weighted random selection")
}

func (pool *serverPool) excludeZeroWeightServers() []*serverutil.Server {
	servers := pool.ServerList
	serverList := make([]*serverutil.Server, 0)
	for _, server := range servers {
		if server.Weight > 0 {
			serverList = append(serverList, server)
		}
	}

	return serverList
}

func excludeUnavailableServers(servers []*serverutil.Server) []*serverutil.Server {
	serverList := make([]*serverutil.Server, 0)
	for _, server := range servers {
		if server.GetAlive() {
			serverList = append(serverList, server)
		}
	}

	return serverList
}

func (pool *serverPool) getWeightedLeastConnectedServer() *serverutil.Server {
	servers := pool.excludeZeroWeightServers()
	serverList := excludeUnavailableServers(servers)
	sort.Slice(serverList, func(i, j int) bool {
		if (serverList[i].ActiveConnections.Value() / serverList[i].Weight) < (serverList[j].ActiveConnections.Value() / serverList[j].Weight) {
			return true
		}
		return false
	})

	return serverList[0]
}

func (pool *serverPool) getLeastConnectedServer() *serverutil.Server {
	serverList := pool.ServerList
	serverList = excludeUnavailableServers(serverList)
	sort.Slice(serverList, func(i, j int) bool {
		return serverList[i].ActiveConnections.Value() < serverList[j].ActiveConnections.Value()
	})
	return serverList[0]
}

func (pool *serverPool) getServerByHash(hash string) (*serverutil.Server, error) {
	serverList := pool.ServerList
	for i := range serverList {
		if serverList[i].ServerHash == hash {
			return serverList[i], nil
		}
	}
	return &serverutil.Server{}, fmt.Errorf("no server found with (%s) hash", hash)
}

func (pool *serverPool) addServer(server *serverutil.Server) {
	pool.ServerList = append(pool.ServerList, server)
}

func (pool *serverPool) clearPool() {
	pool.ServerList = nil
}

func (pool *serverPool) nextPool() int {
	var current int
	if (pool.Current + 1) > (len(pool.ServerList) - 1) {
		pool.Current = 0
		current = pool.Current
	} else {
		pool.Current = pool.Current + 1
		current = pool.Current
	}
	if !pool.ServerList[current].GetAlive() {
		return pool.nextPool()
	}
	return current
}

func compareServerPools(prevPoolHash string, incomingPool []*confg.Endpoint) bool {
	var sumOfServerHash string
	for _, server := range incomingPool {
		serialized, _ := json.Marshal(server)
		sumOfServerHash += string(serialized)
	}
	md := md5.Sum([]byte(sumOfServerHash))
	poolHash := hex.EncodeToString(md[:16])
	if prevPoolHash == poolHash {
		return false
	}
	serverPoolHash = poolHash
	return true
}

func setCookieToResponse(w http.ResponseWriter, hash string) http.ResponseWriter {
	http.SetCookie(w, &http.Cookie{Name: "_balansir_server_hash", Value: hash, MaxAge: configuration.SessionMaxAge})
	return w
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
			endpoint, err := pool.getServerByHash(cookieHash.Value)
			if err == nil {
				endpoint.Proxy.ServeHTTP(w, r)
				return
			}
		}
	}
	switch configuration.Algorithm {
	case "round-robin":
		processingRequests.Add(1)
		index := pool.nextPool()
		endpoint := pool.ServerList[index]
		if configuration.SessionPersistence {
			w = setCookieToResponse(w, endpoint.ServerHash)
		}
		endpoint.Proxy.ServeHTTP(w, r)
		processingRequests.Done()
	case "weighted-round-robin":
		processingRequests.Add(1)
		poolChoice := pool.getPoolChoice()
		endpoint, err := weightedChoice(poolChoice)
		if err != nil {
			log.Println(err)
		}
		if configuration.SessionPersistence {
			w = setCookieToResponse(w, endpoint.ServerHash)
		}
		endpoint.Proxy.ServeHTTP(w, r)
		processingRequests.Done()
	case "least-connections":
		processingRequests.Add(1)
		endpoint := pool.getLeastConnectedServer()
		if configuration.SessionPersistence {
			w = setCookieToResponse(w, endpoint.ServerHash)
		}
		endpoint.ActiveConnections.Add(1)
		endpoint.Proxy.ServeHTTP(w, r)
		endpoint.ActiveConnections.Add(-1)
		processingRequests.Done()
	case "weighted-least-connections":
		processingRequests.Add(1)
		endpoint := pool.getWeightedLeastConnectedServer()
		if configuration.SessionPersistence {
			w = setCookieToResponse(w, endpoint.ServerHash)
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

	if compareServerPools(serverPoolHash, configuration.ServerList) {
		var serverHash string
		wg.Add(len(configuration.ServerList))

		pool.clearPool()

		for index, server := range configuration.ServerList {
			switch configuration.Algorithm {
			case "weighted-round-robin", "weighted-least-connections":
				if server.Weight < 0 {
					log.Fatalf(`Negative weight (%v) is specified for (%s) endpoint in config["server_list"]. Please set it's the weight to 0 if you want to mark it as dead one.`, server.Weight, server.URL)
				} else if server.Weight > 1 {
					log.Fatalf(`Weight can't be greater than 1. You specified (%v) weight for (%s) endpoint in config["server_list"].`, server.Weight, server.URL)
				}
			}

			serverURL, err := url.Parse(configuration.Protocol + "://" + server.URL)
			if err != nil {
				log.Fatal(err)
			}

			proxy := httputil.NewSingleHostReverseProxy(serverURL)
			connections := expvar.NewFloat(helpers.RandomStringBytes(5))

			if configuration.SessionPersistence {
				md := md5.Sum([]byte(serverURL.String()))
				serverHash = hex.EncodeToString(md[:16])
			}

			pool.addServer(&serverutil.Server{
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
			nonZeroServers := pool.excludeZeroWeightServers()
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
			Addr:    ":" + strconv.Itoa(configuration.Port),
			Handler: http.HandlerFunc(helpers.RedirectTLS),
		}
		log.Fatal(server.ListenAndServe())
	}()

	if err := http.ListenAndServeTLS(":"+strconv.Itoa(configuration.TLSPort), configuration.SSLCertificate, configuration.SSLKey, http.HandlerFunc(loadBalance)); err != nil {
		log.Fatalf(`Error starting TLS listener: %s`, err)
	}
}

var configuration confg.Configuration
var pool serverPool
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
			Addr:    ":" + strconv.Itoa(configuration.Port),
			Handler: http.HandlerFunc(loadBalance),
		}
		log.Fatal(server.ListenAndServe())
	}

}
