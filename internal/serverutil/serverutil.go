package serverutil

import (
	"balansir/internal/confg"
	"expvar"
	"log"
	"net"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"
)

type Server struct {
	mux               sync.RWMutex
	URL               *url.URL
	Weight            float64
	Index             int
	ActiveConnections *expvar.Float
	Alive             bool
	Proxy             *httputil.ReverseProxy
	ServerHash        string
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

func (server *Server) CheckAlive(configuration *confg.Configuration) {
	configurationTimeout := configuration.Timeout
	timeout := time.Second * time.Duration(configurationTimeout)
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
