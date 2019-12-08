package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"expvar"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"
)

type Configuration struct {
	ServerList         []*Endpoint `json:"server_list"`
	Protocol           string      `json:"connection_protocol"`
	SSLCertificate     string      `json:"ssl_certificate"`
	SSLKey             string      `json:"ssl_private_key"`
	Port               int         `json:"load_balancer_port"`
	Delay              int         `json:"server_check_timer"`
	SessionPersistence bool        `json:"session_persistence"`
	SessionMaxAge      int         `json:"session_max_age"`
	Timeout            int         `json:"server_check_response_timeout"`
	ProxyMode          string      `json:"proxy_mode"`
	Algorithm          string      `json:"balancing_algorithm"`
}

type Endpoint struct {
	URL    string  `json:"endpoint"`
	Weight float64 `json:"weight"`
}

type Server struct {
	URL               *url.URL
	Weight            float64
	Index             int
	ActiveConnections *expvar.Float
	Alive             bool
	mux               sync.RWMutex
	Proxy             *httputil.ReverseProxy
	ServerHash        string
}

type Choice struct {
	Endpoint *Server
	Weight   int
}

func (server *Server) GetAlive() bool {
	server.mux.RLock()
	defer server.mux.RUnlock()
	status := server.Alive
	return status
}

func (server *Server) SetAlive(status bool) {
	server.mux.Lock()
	defer server.mux.Unlock()
	server.Alive = status
}

func (server *Server) CheckAlive() {
	timeout := time.Second * time.Duration(configuration.Timeout)
	connection, err := net.DialTimeout("tcp", server.URL.Host, timeout)
	if err != nil {
		server.SetAlive(false)
		log.Println("Server is down:", err)
		return
	}
	connection.Close()
	if server.GetAlive() == false {
		log.Println("Server is up:", server.URL.Host)
	}
	server.SetAlive(true)
}

type ServerPool struct {
	ServerList []*Server
	Current    int
}

func (pool *ServerPool) GetPoolChoice() []Choice {
	choice := []Choice{}
	serverList := pool.ExcludeZeroWeightServers()
	serverList = ExcludeUnavailableServers(serverList)
	for _, server := range serverList {
		if server.GetAlive() == true {
			weight := int(server.Weight * 100)
			choice = append(choice, Choice{Weight: weight, Endpoint: server})
		}
	}

	return choice
}

func WeightedChoice(choices []Choice) (*Server, error) {
	rand.Seed(time.Now().UnixNano())
	weightShum := 0
	for _, choice := range choices {
		weightShum += choice.Weight
	}
	randint := rand.Intn(weightShum)

	for _, choice := range choices {
		randint -= choice.Weight
		if randint <= 0 {
			return choice.Endpoint, nil
		}
	}
	return &Server{}, errors.New("no server returned from weighted random selection")
}

func (pool *ServerPool) ExcludeZeroWeightServers() []*Server {
	servers := pool.ServerList
	serverList := make([]*Server, 0)
	for _, server := range servers {
		if server.Weight > 0 {
			serverList = append(serverList, server)
		}
	}

	return serverList
}

func ExcludeUnavailableServers(servers []*Server) []*Server {
	serverList := make([]*Server, 0)
	for _, server := range servers {
		if server.GetAlive() == true {
			serverList = append(serverList, server)
		}
	}

	return serverList
}

func (pool *ServerPool) GetWeightedLeastConnectedServer() *Server {
	servers := pool.ExcludeZeroWeightServers()
	serverList := ExcludeUnavailableServers(servers)
	sort.Slice(serverList, func(i, j int) bool {
		if (serverList[i].ActiveConnections.Value() / serverList[i].Weight) < (serverList[j].ActiveConnections.Value() / serverList[j].Weight) {
			return true
		}
		return false
	})

	return serverList[0]
}

func (pool *ServerPool) GetLeastConnectedServer() *Server {
	serverList := pool.ServerList
	serverList = ExcludeUnavailableServers(serverList)
	sort.Slice(serverList, func(i, j int) bool {
		return serverList[i].ActiveConnections.Value() < serverList[j].ActiveConnections.Value()
	})
	return serverList[0]
}

func (pool *ServerPool) GetServerByHash(hash string) (*Server, error) {
	serverList := pool.ServerList
	for i := range serverList {
		if serverList[i].ServerHash == hash {
			return serverList[i], nil
		}
	}
	return &Server{}, fmt.Errorf("no server found with (%s) hash", hash)
}

func (pool *ServerPool) AddServer(server *Server) {
	pool.ServerList = append(pool.ServerList, server)
}

func (pool *ServerPool) NextPool() int {
	var current int
	if (pool.Current + 1) > (len(pool.ServerList) - 1) {
		pool.Current = 0
		current = pool.Current
	} else {
		pool.Current = pool.Current + 1
		current = pool.Current
	}
	if !pool.ServerList[current].GetAlive() {
		return pool.NextPool()
	}
	return current
}

func addRemoteAddrToRequest(r *http.Request) *http.Request {
	r.Header.Set("X-Forwarded-For", r.RemoteAddr)
	return r
}

func addSessionPersistenceToResponse(w http.ResponseWriter, hash string) http.ResponseWriter {
	http.SetCookie(w, &http.Cookie{Name: "_balansir_server_hash", Value: hash, MaxAge: configuration.SessionMaxAge})
	return w
}

func loadBalance(w http.ResponseWriter, r *http.Request) {
	if configuration.ProxyMode == "transparent" {
		r = addRemoteAddrToRequest(r)
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
		index := pool.NextPool()
		endpoint := pool.ServerList[index]
		if configuration.SessionPersistence {
			w = addSessionPersistenceToResponse(w, endpoint.ServerHash)
		}
		endpoint.Proxy.ServeHTTP(w, r)
	case "weighted-round-robin":
		poolChoice := pool.GetPoolChoice()
		endpoint, err := WeightedChoice(poolChoice)
		if err != nil {
			log.Println(err)
		}
		if configuration.SessionPersistence {
			w = addSessionPersistenceToResponse(w, endpoint.ServerHash)
		}
		endpoint.Proxy.ServeHTTP(w, r)
	case "least-connections":
		endpoint := pool.GetLeastConnectedServer()
		if configuration.SessionPersistence {
			w = addSessionPersistenceToResponse(w, endpoint.ServerHash)
		}
		endpoint.ActiveConnections.Add(1)
		endpoint.Proxy.ServeHTTP(w, r)
		endpoint.ActiveConnections.Add(-1)
	case "weighted-least-connections":
		endpoint := pool.GetWeightedLeastConnectedServer()
		if configuration.SessionPersistence {
			w = addSessionPersistenceToResponse(w, endpoint.ServerHash)
		}
		endpoint.ActiveConnections.Add(1)
		endpoint.Proxy.ServeHTTP(w, r)
		endpoint.ActiveConnections.Add(-1)
	}
}

func serversCheck() {
	timer := time.NewTicker(time.Duration(configuration.Delay) * time.Second)
	for {
		select {
		case <-timer.C:
			for _, server := range pool.ServerList {
				server.CheckAlive()
			}
		}
	}
}

var configuration Configuration
var pool ServerPool

func main() {
	file, err := ioutil.ReadFile("config.json")
	if err != nil {
		log.Fatal(err)
	}
	json.Unmarshal(file, &configuration)

	var serverHash string

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
		connections := expvar.NewFloat("connections-" + strconv.Itoa(index))

		if configuration.SessionPersistence {
			md := md5.Sum([]byte(serverURL.String()))
			serverHash = hex.EncodeToString(md[:16])
		}

		pool.AddServer(&Server{
			URL:               serverURL,
			Weight:            server.Weight,
			ActiveConnections: connections,
			Index:             index,
			Alive:             true,
			Proxy:             proxy,
			ServerHash:        serverHash,
		})
	}

	switch configuration.Algorithm {
	case "weighted-round-robin", "weighted-least-connections":
		nonZeroServers := pool.ExcludeZeroWeightServers()
		if len(nonZeroServers) <= 0 {
			log.Fatalf(`0 weight is specified for all your endpoints in config["server_list"]. Please consider adding at least one endpoint with non-zero weight.`)
		}
	}

	go serversCheck()

	if configuration.Protocol == "https" {
		if err := http.ListenAndServeTLS(":"+strconv.Itoa(configuration.Port), configuration.SSLCertificate, configuration.SSLKey, http.HandlerFunc(loadBalance)); err != nil {
			log.Fatalf(`Error starting TLS http listener: %s`, err)
		}
	} else {
		server := http.Server{
			Addr:    ":" + strconv.Itoa(configuration.Port),
			Handler: http.HandlerFunc(loadBalance),
		}
		log.Fatal(server.ListenAndServe())
	}

}
