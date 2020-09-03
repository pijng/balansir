package poolutil

import (
	"balansir/internal/configutil"
	"balansir/internal/proxyutil"
	"balansir/internal/serverutil"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"expvar"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

//ServerPool ...
type ServerPool struct {
	Guard      sync.WaitGroup
	ServerList []*serverutil.Server
	Current    int64
}

//EndpointChoice ...
type EndpointChoice struct {
	Endpoint *serverutil.Server
	Weight   int
}

var pool *ServerPool
var once sync.Once

//GetPool ...
func GetPool() *ServerPool {
	once.Do(func() {
		pool = &ServerPool{}
	})

	return pool
}

//SetPool ...
func SetPool(newPool *ServerPool) *ServerPool {
	pool = newPool
	return pool
}

//GetPoolChoice ...
func (pool *ServerPool) GetPoolChoice() []EndpointChoice {
	choice := []EndpointChoice{}
	serverList := pool.ExcludeZeroWeightServers()
	serverList = ExcludeUnavailableServers(serverList)
	for _, server := range serverList {
		weight := int(server.Weight * 100)
		choice = append(choice, EndpointChoice{Weight: weight, Endpoint: server})
	}

	return choice
}

//ExcludeZeroWeightServers ...
func (pool *ServerPool) ExcludeZeroWeightServers() []*serverutil.Server {
	servers := pool.ServerList
	serverList := make([]*serverutil.Server, 0)
	for _, server := range servers {
		if server.Weight > 0 {
			serverList = append(serverList, server)
		}
	}

	return serverList
}

//ExcludeUnavailableServers ...
func ExcludeUnavailableServers(servers []*serverutil.Server) []*serverutil.Server {
	serverList := make([]*serverutil.Server, 0)
	for _, server := range servers {
		if server.GetAlive() {
			serverList = append(serverList, server)
		}
	}

	return serverList
}

//WeightedChoice ...
func WeightedChoice(choices []EndpointChoice) (*serverutil.Server, error) {
	rand.Seed(time.Now().UnixNano())
	weightSum := 0
	for _, choice := range choices {
		weightSum += choice.Weight
	}
	randint := rand.Intn(weightSum)

	sort.Slice(choices, func(i, j int) bool {
		return choices[i].Weight > choices[j].Weight
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

//GetWeightedLeastConnectedServer ...
func (pool *ServerPool) GetWeightedLeastConnectedServer() *serverutil.Server {
	servers := pool.ExcludeZeroWeightServers()
	serverList := ExcludeUnavailableServers(servers)
	sort.Slice(serverList, func(i, j int) bool {
		return (serverList[i].ActiveConnections.Value() / serverList[i].Weight) < (serverList[j].ActiveConnections.Value() / serverList[j].Weight)
	})

	return serverList[0]
}

//GetLeastConnectedServer ...
func (pool *ServerPool) GetLeastConnectedServer() *serverutil.Server {
	serverList := pool.ServerList
	serverList = ExcludeUnavailableServers(serverList)
	sort.Slice(serverList, func(i, j int) bool {
		return serverList[i].ActiveConnections.Value() < serverList[j].ActiveConnections.Value()
	})
	return serverList[0]
}

//GetServerByHash ...
func (pool *ServerPool) GetServerByHash(hash string) (*serverutil.Server, error) {
	serverList := pool.ServerList
	for i := range serverList {
		if serverList[i].ServerHash == hash {
			return serverList[i], nil
		}
	}
	return &serverutil.Server{}, fmt.Errorf("no server found with (%s) hash", hash)
}

//AddServer ...
func (pool *ServerPool) AddServer(server *serverutil.Server) {
	pool.ServerList = append(pool.ServerList, server)
}

//ClearPool ...
func (pool *ServerPool) ClearPool() {
	pool.ServerList = nil
}

//NextPool ...
func (pool *ServerPool) NextPool() int {
	return int(atomic.AddInt64(&pool.Current, 1) % int64(len(pool.ServerList)))
}

//RedefineServerPool ...
func RedefineServerPool(configuration *configutil.Configuration, serverPoolGuard *sync.WaitGroup) (*ServerPool, error) {
	var serverHash string
	serverPoolGuard.Add(len(configuration.ServerList))

	newPool := &ServerPool{}
	for index, server := range configuration.ServerList {
		switch configuration.Algorithm {
		case "weighted-round-robin", "weighted-least-connections":
			if server.Weight < 0 {
				return nil, fmt.Errorf(`negative weight (%v) is specified for (%s) endpoint in config["server_list"]. Please set it's the weight to 0 if you want to mark it as dead one`, server.Weight, server.URL)
			} else if server.Weight > 1 {
				return nil, fmt.Errorf(`weight can't be greater than 1. You specified (%v) weight for (%s) endpoint in config["server_list"]`, server.Weight, server.URL)
			}
		}

		serverURL, err := url.Parse(configuration.Protocol + "://" + strings.TrimSpace(server.URL))
		if err != nil {
			return nil, err
		}

		proxy := httputil.NewSingleHostReverseProxy(serverURL)
		proxy.Transport = &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   time.Duration(configuration.WriteTimeout) * time.Second,
				KeepAlive: time.Duration(configuration.ReadTimeout) * time.Second,
			}).DialContext,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
		}

		proxy.ModifyResponse = proxyutil.ModifyResponse
		proxy.ErrorHandler = proxyutil.ErrorHandler

		connections := expvar.NewFloat(randomStringBytes(5))

		if configuration.SessionPersistence {
			md := md5.Sum([]byte(serverURL.String()))
			serverHash = hex.EncodeToString(md[:16])
		}

		newPool.AddServer(&serverutil.Server{
			URL:               serverURL,
			Weight:            server.Weight,
			ActiveConnections: connections,
			Index:             index,
			Alive:             true,
			Proxy:             proxy,
			ServerHash:        serverHash,
		})
		serverPoolGuard.Done()
	}

	switch configuration.Algorithm {
	case "weighted-round-robin", "weighted-least-connections":
		nonZeroServers := newPool.ExcludeZeroWeightServers()
		if len(nonZeroServers) <= 0 {
			return nil, fmt.Errorf(`0 weight is specified for all your endpoints in config["server_list"]. Please consider adding at least one endpoint with non-zero weight`)
		}
	}

	return newPool, nil
}

func randomStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Int63()%int64(len(letterBytes))]
	}
	return string(b)
}
