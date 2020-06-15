package metricsutil

import (
	"balansir/internal/cacheutil"
	"balansir/internal/configutil"
	"balansir/internal/logutil"
	"balansir/internal/metricsutil/pstats"
	"balansir/internal/rateutil"
	"balansir/internal/serverutil"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"
	"runtime"
	"sync/atomic"
	"time"
)

var mem runtime.MemStats

type objects struct {
	rateCounter   *rateutil.Rate
	configuration *configutil.Configuration
	servers       []*serverutil.Server
	cache         *cacheutil.CacheCluster
}

//Stats ...
type Stats struct {
	Timestamp            int64       `json:"timestamp"`
	RequestsPerSecond    float64     `json:"requests_per_second"`
	AverageResponseTime  float64     `json:"average_response_time"`
	MemoryUsage          int64       `json:"memory_usage"`
	ErrorsCount          int64       `json:"errors_count"`
	Port                 int         `json:"http_port"`
	TLSPort              int         `json:"https_port"`
	Endpoints            []*endpoint `json:"endpoints"`
	TransparentProxyMode bool        `json:"transparent_proxy_mode"`
	Algorithm            string      `json:"balancing_algorithm"`
	Cache                bool        `json:"cache"`
	CacheInfo            cacheInfo   `json:"cache_info"`
}

type endpoint struct {
	URL               string  `json:"url"`
	Active            bool    `json:"active"`
	Weight            float64 `json:"weight"`
	ActiveConnections float64 `json:"active_connections"`
	ServerHash        string  `json:"server_hash"`
}

type cacheInfo struct {
	HitRatio     float64 `json:"hit_ratio"`
	ShardsAmount int     `json:"shards_amount"`
	ShardSize    int     `json:"shard_size_mb"`
	Hits         int64   `json:"hits"`
	Misses       int64   `json:"misses"`
}

//MetrictStats ...
func MetrictStats(w http.ResponseWriter, r *http.Request) {
	val := GetBalansirStats()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(val); err != nil {
		logutil.Warning(err)
	}
}

var metrics *objects

//AssignMetricsObjects ...
func AssignMetricsObjects(rc *rateutil.Rate, c *configutil.Configuration, s []*serverutil.Server, cc *cacheutil.CacheCluster) {
	metrics = &objects{
		rateCounter:   rc,
		configuration: c,
		servers:       s,
		cache:         cc,
	}
}

//GetBalansirStats ...
func GetBalansirStats() Stats {
	runtime.ReadMemStats(&mem)
	endpoints := make([]*endpoint, len(metrics.servers))
	for i, server := range metrics.servers {
		server.Mux.RLock()
		endpoints[i] = &endpoint{
			server.URL.String(),
			server.Alive,
			server.Weight,
			server.ActiveConnections.Value(),
			server.ServerHash,
		}
		server.Mux.RUnlock()
	}
	return Stats{
		Timestamp:            time.Now().Unix() * 1000,
		RequestsPerSecond:    metrics.rateCounter.RateValue(),
		AverageResponseTime:  metrics.rateCounter.ResponseValue(),
		MemoryUsage:          getRSSUsage(),
		ErrorsCount:          getErrorsCount(),
		Port:                 metrics.configuration.Port,
		TLSPort:              metrics.configuration.TLSPort,
		Endpoints:            endpoints,
		TransparentProxyMode: metrics.configuration.TransparentProxyMode,
		Algorithm:            metrics.configuration.Algorithm,
		Cache:                metrics.configuration.Cache,
		CacheInfo: cacheInfo{
			HitRatio:     metrics.cache.GetHitRatio(),
			ShardsAmount: metrics.cache.ShardsAmount,
			ShardSize:    metrics.cache.ShardMaxSize,
			Hits:         atomic.LoadInt64(&metrics.cache.Hits),
			Misses:       atomic.LoadInt64(&metrics.cache.Misses),
		},
	}
}

//Metrics ...
func Metrics(w http.ResponseWriter, r *http.Request) {
	wd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	tmpl := template.Must(template.ParseFiles(wd + "/content/templates/index.html"))
	err = tmpl.Execute(w, nil)
	if err != nil {
		logutil.Error(err)
	}
}

func getRSSUsage() int64 {
	// https://utcc.utoronto.ca/~cks/space/blog/programming/GoNoMemoryFreeing
	// Needs additional investigation, since go MAY return RSS back to OS
	// return int64(mem.HeapInuse) / 1024 / 1024
	// return int64(mem.HeapSys-mem.HeapIdle) / 1024 / 1024

	var rss int64
	var err error

	os := runtime.GOOS
	switch os {
	case "darwin":
		rss, err = pstats.GetRSSInfoDarwin()
		if err != nil {
			logutil.Warning(err)
		}
	case "linux":
		rss, err = pstats.GetRSSInfoLinux()
		if err != nil {
			logutil.Warning(err)
		}
	default:
		rss = 0
	}

	return rss
}

func getErrorsCount() int64 {
	return 0
}
