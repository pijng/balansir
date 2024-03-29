package metricsutil

import (
	"balansir/internal/cacheutil"
	"balansir/internal/configutil"
	"balansir/internal/logutil"
	"balansir/internal/metricsutil/pstats"
	"balansir/internal/poolutil"
	"balansir/internal/rateutil"
	"balansir/internal/serverutil"
	"balansir/internal/statusutil"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

var mem runtime.MemStats

type objects struct {
	rateCounter   *rateutil.Rate
	configuration *configutil.Configuration
	servers       []*serverutil.Server
	cache         *cacheutil.CacheCluster
	statusCodes   *statusutil.StatusCodes
}

//Stats ...
type Stats struct {
	Timestamp           int64         `json:"timestamp"`
	RequestsPerSecond   float64       `json:"requests_per_second"`
	AverageResponseTime float64       `json:"average_response_time"`
	MemoryUsage         int64         `json:"memory_usage"`
	ErrorsCount         int64         `json:"errors_count"`
	Port                int           `json:"http_port"`
	TLSPort             int           `json:"https_port"`
	Endpoints           []*endpoint   `json:"endpoints"`
	TransparentProxy    bool          `json:"transparent_proxy"`
	Algorithm           string        `json:"balancing_algorithm"`
	Cache               bool          `json:"cache"`
	CacheInfo           cacheInfo     `json:"cache_info"`
	StatusCodes         map[int]int64 `json:"status_codes"`
}

type endpoint struct {
	URL               string  `json:"url"`
	Active            bool    `json:"active"`
	Weight            float64 `json:"weight"`
	ActiveConnections int64   `json:"active_connections"`
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
	val := getBalansirStats()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(&val); err != nil {
		logutil.Warning(err)
	}
}

//CollectedStats ...
func CollectedStats(w http.ResponseWriter, r *http.Request) {
	wd, err := os.Getwd()
	if err != nil {
		logutil.Fatal(err)
	}
	http.ServeFile(w, r, wd+fmt.Sprintf("/%s", logutil.StatsPath))
}

//CollectedLogs ...
func CollectedLogs(w http.ResponseWriter, r *http.Request) {
	file, _ := os.OpenFile(logutil.JSONPath, os.O_RDWR, 0644)
	defer file.Close()
	bytes, _ := ioutil.ReadAll(file)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, err := w.Write(bytes)
	if err != nil {
		logutil.Warning(fmt.Errorf("Error writing collected logs to dashboard: %w", err))
	}
}

var metrics *objects
var once sync.Once

//InitMetricsMeta ...
func InitMetricsMeta(rc *rateutil.Rate, c *configutil.Configuration, s []*serverutil.Server, sc *statusutil.StatusCodes) {
	cc := cacheutil.GetCluster()
	metrics = &objects{
		rateCounter:   rc,
		configuration: c,
		servers:       s,
		cache:         cc,
		statusCodes:   sc,
	}

	once.Do(func() {
		go func() {
			timer := time.NewTicker(1 * time.Second)
			for {
				<-timer.C
				logutil.Stats(getBalansirStats())
			}
		}()
	})
}

func getBalansirStats() *Stats {
	runtime.ReadMemStats(&mem)
	endpoints := make([]*endpoint, len(metrics.servers))
	for i, server := range metrics.servers {
		server.Mux.RLock()
		endpoints[i] = &endpoint{
			server.URL.String(),
			server.Alive,
			server.Weight,
			server.GetActiveConnections(),
			server.ServerHash,
		}
		server.Mux.RUnlock()
	}

	stats := Stats{
		Timestamp:           time.Now().Unix() * 1000,
		RequestsPerSecond:   metrics.rateCounter.RequestsPerSecond(),
		AverageResponseTime: metrics.rateCounter.AverageResponseTime(),
		MemoryUsage:         getRSSUsage(),
		ErrorsCount:         getErrorsCount(),
		Port:                metrics.configuration.Port,
		TLSPort:             metrics.configuration.TLSPort,
		Endpoints:           endpoints,
		TransparentProxy:    metrics.configuration.TransparentProxy,
		Algorithm:           metrics.configuration.Algorithm,
		Cache:               metrics.configuration.Cache.Enabled,
		StatusCodes:         metrics.statusCodes.GetStatuses(),
	}

	cache := cacheutil.GetCluster()
	if metrics.configuration.Cache.Enabled && cache != nil {
		stats.CacheInfo = cacheInfo{
			HitRatio:     metrics.cache.GetHitRatio(),
			ShardsAmount: metrics.cache.ShardsAmount,
			ShardSize:    metrics.cache.ShardSize,
			Hits:         atomic.LoadInt64(&metrics.cache.Hits),
			Misses:       atomic.LoadInt64(&metrics.cache.Misses),
		}
	}

	return &stats
}

//Metrics ...
func Metrics(w http.ResponseWriter, r *http.Request) {
	wd, err := os.Getwd()
	if err != nil {
		logutil.Fatal(err)
	}
	tmpl := template.Must(template.ParseFiles(wd + "/content/templates/index.html"))
	err = tmpl.Execute(w, nil)
	if err != nil {
		logutil.Error(err)
	}
}

//MetricsPolling ...
func MetricsPolling() {
	configuration := configutil.GetConfig()
	pool := poolutil.GetPool()
	rateCounter := rateutil.GetRateCounter()
	statusCodes := statusutil.GetStatusCodes()
	InitMetricsMeta(rateCounter, configuration, pool.ServerList, statusCodes)
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
