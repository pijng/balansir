package metricsutil

import (
	"balansir/internal/confg"
	"balansir/internal/rateutil"
	"balansir/internal/serverutil"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"
	"syscall"
	"time"
)

var mem syscall.Rusage

//Stats ...
type Stats struct {
	Timestamp           int64       `json:"timestamp"`
	RequestsPerSecond   float64     `json:"requests_per_second"`
	AverageResponseTime float64     `json:"average_response_time"`
	MemoryUsage         int64       `json:"memory_usage"`
	ErrorsCount         int64       `json:"errors_count"`
	Port                int         `json:"http_port"`
	TLSPort             int         `json:"https_port"`
	Endpoints           []*endpoint `json:"endpoints"`
	ProxyMode           string      `json:"proxy_mode"`
	Algorithm           string      `json:"balancing_algorithm"`
	Cache               bool        `json:"cache"`
}

type endpoint struct {
	URL               string  `json:"url"`
	Weight            float64 `json:"weight"`
	ActiveConnections float64 `json:"active_connections"`
	ServerHash        string  `json:"server_hash"`
}

//MetricsPasser ...
type MetricsPasser struct {
	MetricsChan chan Stats
}

//MetrictStats ...
func (mp *MetricsPasser) MetrictStats(w http.ResponseWriter, r *http.Request) {
	val := <-mp.MetricsChan
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(val)
}

//GetBalansirStats ...
func GetBalansirStats(rateCounter *rateutil.Rate, configuration *confg.Configuration, servers []*serverutil.Server) Stats {
	endpoints := make([]*endpoint, len(servers))
	for i, server := range servers {
		endpoints[i] = &endpoint{
			server.URL.String(),
			server.Weight,
			server.ActiveConnections.Value(),
			server.ServerHash,
		}
	}
	return Stats{
		Timestamp:           time.Now().Unix() * 1000,
		RequestsPerSecond:   rateCounter.RateValue(),
		AverageResponseTime: rateCounter.ResponseValue(),
		MemoryUsage:         getRSSUsage(),
		ErrorsCount:         getErrorsCount(),
		Port:                configuration.Port,
		TLSPort:             configuration.TLSPort,
		Endpoints:           endpoints,
		ProxyMode:           configuration.ProxyMode,
		Algorithm:           configuration.Algorithm,
		Cache:               configuration.Cache,
	}
}

//Metrics ...
func Metrics(w http.ResponseWriter, r *http.Request) {
	wd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	tmpl := template.Must(template.ParseFiles(wd + "/content/templates/index.html"))
	tmpl.Execute(w, nil)
}

func getRSSUsage() int64 {
	syscall.Getrusage(syscall.RUSAGE_SELF, &mem)
	return mem.Maxrss / 1024 / 1024
}

func getErrorsCount() int64 {
	return 0
}
