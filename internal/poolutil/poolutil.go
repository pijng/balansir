package poolutil

import (
	"balansir/internal/serverutil"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"
)

//ServerPool ...
type ServerPool struct {
	mux        sync.RWMutex
	ServerList []*serverutil.Server
	Current    int
}

//EndpointChoice ...
type EndpointChoice struct {
	Endpoint *serverutil.Server
	Weight   int
}

//GetPoolChoice ...
func (pool *ServerPool) GetPoolChoice() ([]EndpointChoice, error) {
	choice := []EndpointChoice{}
	serverList := pool.ExcludeZeroWeightServers()
	serverList = ExcludeUnavailableServers(serverList)
	if len(serverList) == 0 {
		return nil, errors.New("all servers are down")
	}
	for _, server := range serverList {
		weight := int(server.Weight * 100)
		choice = append(choice, EndpointChoice{Weight: weight, Endpoint: server})
	}

	return choice, nil
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
func (pool *ServerPool) GetWeightedLeastConnectedServer() (*serverutil.Server, error) {
	servers := pool.ExcludeZeroWeightServers()
	serverList := ExcludeUnavailableServers(servers)
	if len(serverList) == 0 {
		return nil, errors.New("all servers are down")
	}
	sort.Slice(serverList, func(i, j int) bool {
		return (serverList[i].ActiveConnections.Value() / serverList[i].Weight) < (serverList[j].ActiveConnections.Value() / serverList[j].Weight)
	})

	return serverList[0], nil
}

//GetLeastConnectedServer ...
func (pool *ServerPool) GetLeastConnectedServer() (*serverutil.Server, error) {
	serverList := pool.ServerList
	serverList = ExcludeUnavailableServers(serverList)
	if len(serverList) == 0 {
		return nil, errors.New("all servers are down")
	}
	sort.Slice(serverList, func(i, j int) bool {
		return serverList[i].ActiveConnections.Value() < serverList[j].ActiveConnections.Value()
	})
	return serverList[0], nil
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
func (pool *ServerPool) NextPool() (int, error) {
	var current int
	pool.mux.Lock()
	defer pool.mux.Unlock()
	serverList := ExcludeUnavailableServers(pool.ServerList)
	if len(serverList) == 0 {
		return 0, errors.New("all servers are down")
	}
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
	return current, nil
}
