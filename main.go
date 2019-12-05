package main

import (
	"encoding/json"
	"expvar"
	"io/ioutil"
	"log"
	"math"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/jmcvetta/randutil"
)

type Configuration struct {
	ServerList []*Endpoint `json:"server_list"`
	Protocol   string      `json:"ecosystem_protocol"`
	Port       int         `json:"load_balancer_port"`
	Delay      int         `json:"server_check_delay"`
	Timeout    int         `json:"server_check_timeout"`
	ProxyMode  string      `json:"proxy_mode"`
	Algorithm  string      `json:"balancing_algorithm"`
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
}

func (server *Server) GetAlive() bool {
	server.mux.RLock()
	status := server.Alive
	server.mux.RUnlock()
	return status
}

func (server *Server) SetAlive(status bool) {
	server.mux.Lock()
	server.Alive = status
	server.mux.Unlock()
}

func (server *Server) CheckAlive() {
	timeout := time.Second * time.Duration(configuration.Timeout)
	connection, err := net.DialTimeout("tcp", server.URL.Host, timeout)
	if err != nil {
		server.SetAlive(false)
		log.Println("Server is down: ", err)
		return
	}
	connection.Close()
	server.SetAlive(true)
}

type ServerPool struct {
	ServerList []*Server
	Current    int
}

func (pool *ServerPool) GetPoolChoice() []randutil.Choice {
	choice := []randutil.Choice{}
	serverList := pool.ExcludeZeroWeightServers()
	for _, server := range serverList {
		if server.GetAlive() {
			weight := int(server.Weight * 100)
			if (weight > 0) && (weight < 1) {
				weight = 1
			}
			choice = append(choice, randutil.Choice{Weight: weight, Item: server.Index})
		}
	}

	return choice
}

func (pool *ServerPool) ExcludeZeroWeightServers() []*Server {
	serverList := pool.ServerList
	k := 0
	for _, server := range serverList {
		if server.Weight > 0 {
			serverList[k] = server
			k++
		}
	}
	serverList = serverList[:k]

	return serverList
}

func (pool *ServerPool) GetWeightedLeastConnectedServer() *Server {
	serverList := pool.ExcludeZeroWeightServers()
	sort.Slice(serverList, func(i, j int) bool {
		if (math.Max(serverList[i].ActiveConnections.Value(), 1) / serverList[i].Weight) < (math.Max(serverList[j].ActiveConnections.Value(), 1) / serverList[j].Weight) {
			return true
		}
		return false
	})

	return serverList[0]
}

func (pool *ServerPool) GetLeastConnectedServer() *Server {
	serverList := pool.ServerList
	sort.Slice(serverList, func(i, j int) bool {
		return serverList[i].ActiveConnections.Value() < serverList[j].ActiveConnections.Value()
	})
	return serverList[0]
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

func loadBalance(w http.ResponseWriter, r *http.Request) {
	switch configuration.Algorithm {
	case "round-robin":
		index := pool.NextPool()
		endpoint := pool.ServerList[index]
		endpoint.Proxy.ServeHTTP(w, r)
	case "weighted-round-robin":
		poolChoice := pool.GetPoolChoice()
		server, _ := randutil.WeightedChoice(poolChoice)
		index := server.Item.(int)
		endpoint := pool.ServerList[index]
		endpoint.Proxy.ServeHTTP(w, r)
	case "least-connections":
		endpoint := pool.GetLeastConnectedServer()
		endpoint.ActiveConnections.Add(1)
		endpoint.Proxy.ServeHTTP(w, r)
		endpoint.ActiveConnections.Add(-1)
	case "weighted-least-connections":
		endpoint := pool.GetWeightedLeastConnectedServer()
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

	for index, server := range configuration.ServerList {
		switch configuration.Algorithm {
		case "weighted-round-robin", "weighted-least-connections":
			if server.Weight < 0 {
				log.Fatalf(`Negative weight is specified for "%s" endpoint in config["server_list"]. If you want to mark it as dead one, please set it's the weight to 0.`, server.URL)
				os.Exit(1)
			}
		}

		serverURL, err := url.Parse(configuration.Protocol + "://" + server.URL)
		if err != nil {
			log.Fatal(err)
		}
		proxy := httputil.NewSingleHostReverseProxy(serverURL)

		connections := expvar.NewFloat("connections-" + strconv.Itoa(index))

		pool.AddServer(&Server{
			URL:               serverURL,
			Weight:            server.Weight,
			ActiveConnections: connections,
			Index:             index,
			Alive:             true,
			Proxy:             proxy,
		})
	}

	switch configuration.Algorithm {
	case "weighted-round-robin", "weighted-least-connections":
		nonZeroServers := pool.ExcludeZeroWeightServers()
		if len(nonZeroServers) <= 0 {
			log.Fatalf(`0 weight is specified for all your endpoints in config["server_list"]. Please consider adding at least one endpoint with non-zero weight.`)
			os.Exit(1)
		}
	}

	server := http.Server{
		Addr:    ":" + strconv.Itoa(configuration.Port),
		Handler: http.HandlerFunc(loadBalance),
	}

	go serversCheck()

	server.ListenAndServe()
}
