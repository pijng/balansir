package serverutil

import (
	"balansir/internal/logutil"
	"expvar"
	"fmt"
	"net"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"
)

//Server ...
type Server struct {
	URL               *url.URL
	Weight            float64
	Index             int
	ActiveConnections *expvar.Float
	Alive             bool
	Proxy             *httputil.ReverseProxy
	ServerHash        string
	Mux               sync.RWMutex
}

//GetAlive ...
func (server *Server) GetAlive() bool {
	server.Mux.RLock()
	defer server.Mux.RUnlock()
	status := server.Alive
	return status
}

//SetAlive ...
func (server *Server) SetAlive(status bool) {
	server.Mux.Lock()
	defer server.Mux.Unlock()
	server.Alive = status
}

//CheckAlive ...
func (server *Server) CheckAlive(tcpTimeout *int) {
	configurationTimeout := *tcpTimeout
	timeout := time.Second * time.Duration(configurationTimeout)
	connection, err := net.DialTimeout("tcp", server.URL.Host, timeout)
	if err != nil {
		server.SetAlive(false)
		logutil.Warning(fmt.Sprintf("Server is down: %v", err))
		return
	}
	connection.Close()
	if !server.GetAlive() {
		logutil.Notice(fmt.Sprintf("Server is up: %v", server.URL.Host))
	}
	server.SetAlive(true)
}
