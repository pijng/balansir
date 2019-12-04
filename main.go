package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"sync"
	"time"
)

type Configuration struct {
	ServerList []string `json:"server_list"`
	Protocol   string   `json:"ecosystem_protocol"`
	Port       int      `json:"load_balancer_port"`
	Timeout    int      `json:"server_check_timeout"`
}

type Server struct {
	URL   *url.URL
	Alive bool
	mux   sync.RWMutex
	Proxy *httputil.ReverseProxy
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

func (pool *ServerPool) AddServer(server *Server) {
	pool.ServerList = append(pool.ServerList, server)
}

func (pool *ServerPool) NextPool() int {
	current := 0
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
	index := pool.NextPool()
	pool.ServerList[index].Proxy.ServeHTTP(w, r)
}

func serversCheck() {
	timer := time.NewTicker(time.Second * 10)
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

	for _, serverURL := range configuration.ServerList {
		serverURL, err := url.Parse(configuration.Protocol + "://" + serverURL)
		if err != nil {
			log.Fatal(err)
		}
		proxy := httputil.NewSingleHostReverseProxy(serverURL)

		pool.AddServer(&Server{
			URL:   serverURL,
			Alive: true,
			Proxy: proxy,
		})
	}

	server := http.Server{
		Addr:    ":" + strconv.Itoa(configuration.Port),
		Handler: http.HandlerFunc(loadBalance),
	}

	go serversCheck()

	server.ListenAndServe()
}
