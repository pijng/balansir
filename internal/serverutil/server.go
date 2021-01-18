package serverutil

import (
	"balansir/internal/logutil"
	"fmt"
	"net"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

//Server ...
type Server struct {
	URL               *url.URL
	Weight            float64
	Index             int
	ActiveConnections int64
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
func (server *Server) CheckAlive(tcpTimeout *int) bool {
	configurationTimeout := *tcpTimeout
	timeout := time.Second * time.Duration(configurationTimeout)
	connection, err := net.DialTimeout("tcp", server.URL.Host, timeout)
	if err != nil {
		server.SetAlive(false)
		logutil.Warning(fmt.Sprintf("Server is down: %v", err))
		return false
	}
	connection.Close()
	if !server.GetAlive() {
		logutil.Notice(fmt.Sprintf("Server is up: %v", server.URL.Host))
	}
	server.SetAlive(true)
	return true
}

//IncreaseActiveConnections ...
func (server *Server) IncreaseActiveConnections() {
	atomic.AddInt64(&server.ActiveConnections, 1)
}

//DecreaseActiveConnections ...
func (server *Server) DecreaseActiveConnections() {
	atomic.AddInt64(&server.ActiveConnections, -1)
}

//GetActiveConnections ...
func (server *Server) GetActiveConnections() int64 {
	return atomic.LoadInt64(&server.ActiveConnections)
}
