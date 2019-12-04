package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
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
	ActiveConnections int
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
	for _, server := range pool.ServerList {
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

func (pool *ServerPool) GetWeightedLeastConnectedServer() *Server {
	serverList := pool.ServerList
	sort.Slice(serverList, func(i, j int) bool {
		if (math.Max(float64(serverList[i].ActiveConnections), 1) / serverList[i].Weight) < (math.Max(float64(serverList[j].ActiveConnections), 1) / serverList[j].Weight) {
			return true
		}
		return false
	})

	fmt.Println(serverList[0])
	return serverList[0]
}

func (pool *ServerPool) GetLeastConnectedServer() *Server {
	serverList := pool.ServerList
	sort.Slice(serverList, func(i, j int) bool {
		return serverList[i].ActiveConnections < serverList[j].ActiveConnections
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
		endpoint.ActiveConnections = endpoint.ActiveConnections + 1
		endpoint.Proxy.ServeHTTP(w, r)
		endpoint.ActiveConnections = endpoint.ActiveConnections - 1
	case "weighted-least-connections":
		endpoint := pool.GetWeightedLeastConnectedServer()
		endpoint.ActiveConnections = endpoint.ActiveConnections + 1
		endpoint.Proxy.ServeHTTP(w, r)
		endpoint.ActiveConnections = endpoint.ActiveConnections - 1
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
		serverURL, err := url.Parse(configuration.Protocol + "://" + server.URL)
		if err != nil {
			log.Fatal(err)
		}
		proxy := httputil.NewSingleHostReverseProxy(serverURL)

		pool.AddServer(&Server{
			URL:               serverURL,
			Weight:            server.Weight,
			ActiveConnections: 0,
			Index:             index,
			Alive:             true,
			Proxy:             proxy,
		})
	}

	server := http.Server{
		Addr:    ":" + strconv.Itoa(configuration.Port),
		Handler: http.HandlerFunc(loadBalance),
	}

	go serversCheck()

	server.ListenAndServe()
}
