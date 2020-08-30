package balanceutil

import (
	"balansir/internal/configutil"
	"balansir/internal/helpers"
	"balansir/internal/logutil"
	"balansir/internal/poolutil"
	"net/http"
)

const (
	//RoundRobinType ...
	RoundRobinType = "round-robin"
	//WeightedRoundRobinType ...
	WeightedRoundRobinType = "weighted-round-robin"
	//LeastConnectionsType ...
	LeastConnectionsType = "least-connections"
	//WeightedLeastConnectionsType ...
	WeightedLeastConnectionsType = "weighted-least-connections"
)

//RoundRobin ...
func RoundRobin(w http.ResponseWriter, r *http.Request) {
	pool := poolutil.GetPool()
	index := pool.NextPool()
	endpoint := pool.ServerList[index]
	configuration := configutil.GetConfig()

	if configuration.SessionPersistence {
		w = helpers.SetCookieToResponse(w, endpoint.ServerHash, configuration)
	}
	helpers.ServeDistributor(endpoint, configuration.Timeout, w, r)
}

//WeightedRoundRobin ...
func WeightedRoundRobin(w http.ResponseWriter, r *http.Request) {
	pool := poolutil.GetPool()
	poolChoice := pool.GetPoolChoice()
	endpoint, err := poolutil.WeightedChoice(poolChoice)
	configuration := configutil.GetConfig()

	if err != nil {
		logutil.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if configuration.SessionPersistence {
		w = helpers.SetCookieToResponse(w, endpoint.ServerHash, configuration)
	}
	helpers.ServeDistributor(endpoint, configuration.Timeout, w, r)
}

//LeastConnections ...
func LeastConnections(w http.ResponseWriter, r *http.Request) {
	pool := poolutil.GetPool()
	endpoint := pool.GetLeastConnectedServer()
	configuration := configutil.GetConfig()

	if configuration.SessionPersistence {
		w = helpers.SetCookieToResponse(w, endpoint.ServerHash, configuration)
	}
	endpoint.ActiveConnections.Add(1)
	helpers.ServeDistributor(endpoint, configuration.Timeout, w, r)
	endpoint.ActiveConnections.Add(-1)
}

//WeightedLeastConnections ...
func WeightedLeastConnections(w http.ResponseWriter, r *http.Request) {
	pool := poolutil.GetPool()
	endpoint := pool.GetWeightedLeastConnectedServer()
	configuration := configutil.GetConfig()

	if configuration.SessionPersistence {
		w = helpers.SetCookieToResponse(w, endpoint.ServerHash, configuration)
	}
	endpoint.ActiveConnections.Add(1)
	helpers.ServeDistributor(endpoint, configuration.Timeout, w, r)
	endpoint.ActiveConnections.Add(-1)
}
