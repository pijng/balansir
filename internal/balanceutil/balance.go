package balanceutil

import (
	"balansir/internal/cacheutil"
	"balansir/internal/configutil"
	"balansir/internal/helpers"
	"balansir/internal/limitutil"
	"balansir/internal/logutil"
	"balansir/internal/metricsutil"
	"balansir/internal/poolutil"
	"balansir/internal/rateutil"
	"balansir/internal/staticutil"
	"net/http"
	"time"
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
		w = helpers.SetSession(w, endpoint.ServerHash, configuration)
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
		w = helpers.SetSession(w, endpoint.ServerHash, configuration)
	}
	helpers.ServeDistributor(endpoint, configuration.Timeout, w, r)
}

//LeastConnections ...
func LeastConnections(w http.ResponseWriter, r *http.Request) {
	pool := poolutil.GetPool()
	endpoint := pool.GetLeastConnectedServer()
	configuration := configutil.GetConfig()

	if configuration.SessionPersistence {
		w = helpers.SetSession(w, endpoint.ServerHash, configuration)
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
		w = helpers.SetSession(w, endpoint.ServerHash, configuration)
	}
	endpoint.ActiveConnections.Add(1)
	helpers.ServeDistributor(endpoint, configuration.Timeout, w, r)
	endpoint.ActiveConnections.Add(-1)
}

//NewServeMux ...
func NewServeMux() *http.ServeMux {
	sm := http.NewServeMux()
	metricsutil.MetricsPolling()
	sm.HandleFunc("/", LoadBalance)
	sm.HandleFunc("/balansir/metrics", metricsutil.Metrics)
	sm.HandleFunc("/balansir/logs", metricsutil.Metrics)
	sm.HandleFunc("/balansir/logs/collected_logs", metricsutil.CollectedLogs)
	sm.HandleFunc("/balansir/metrics/stats", metricsutil.MetrictStats)
	sm.HandleFunc("/balansir/metrics/collected_stats", metricsutil.CollectedStats)
	sm.Handle("/content/", http.StripPrefix("/content/", http.FileServer(http.Dir("content"))))
	return sm
}

//LoadBalance ...
func LoadBalance(w http.ResponseWriter, r *http.Request) {
	configuration := configutil.GetConfig()
	configuration.Guard.Wait()

	if configuration.ServeStatic {
		if staticutil.IsStatic(r.URL.Path) {
			err := staticutil.TryServeStatic(w, r)
			if err == nil {
				return
			}
			logutil.Warning(err)
		}
	}

	if configuration.Cache.Enabled {
		if err := cacheutil.TryServeFromCache(w, r); err == nil {
			return
		}
	}

	if configuration.RateLimit {
		ip := helpers.ReturnIPFromHost(r.RemoteAddr)
		visitors := limitutil.GetLimiter()
		limiter := visitors.GetVisitor(ip, configuration)
		if !limiter.Allow() {
			http.Error(w, http.StatusText(429), http.StatusTooManyRequests)
			return
		}
	}

	pool := poolutil.GetPool()
	availableServers := poolutil.ExcludeUnavailableServers(pool.ServerList)
	if len(availableServers) == 0 {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if r.Header.Get("X-Balansir-Background-Update") == "" {
		rateCounter := rateutil.GetRateCounter()
		rateCounter.HitRequest()
		rtStart := time.Now()
		defer rateCounter.CommitResponseTime(rtStart)
	}

	if configuration.TransparentProxy {
		r = helpers.AddRemoteAddrToRequest(r)
	}

	if configuration.SessionPersistence {
		serverHash, _ := r.Cookie("X-Balansir-Server-Hash")
		if serverHash != nil {
			endpoint, err := pool.GetServerByHash(serverHash.Value)
			if err != nil {
				// If there is no server for the given hash in the pool – just warn about it and
				// continue to algorithm switching to choose a new server.
				// Also, consider disabling this behavior with configuration.
				logutil.Warning(err)
			} else {
				w = helpers.SetSession(w, endpoint.ServerHash, configuration)
				helpers.ServeDistributor(endpoint, configuration.Timeout, w, r)
				return
			}
		}
	}

	switch configuration.Algorithm {
	case RoundRobinType:
		RoundRobin(w, r)

	case WeightedRoundRobinType:
		WeightedRoundRobin(w, r)

	case LeastConnectionsType:
		LeastConnections(w, r)

	case WeightedLeastConnectionsType:
		WeightedLeastConnections(w, r)
	}
}
