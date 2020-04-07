package metricsutil

import (
	"balansir/internal/rateutil"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"
)

//Stats ...
type Stats struct {
	RequestsPerSecond   float64 `json:"requests_per_second"`
	AverageResponseTime float64 `json:"average_response_time"`
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
func GetBalansirStats(rateCounter *rateutil.Rate) Stats {
	return Stats{
		RequestsPerSecond:   rateCounter.RateValue(),
		AverageResponseTime: rateCounter.ResponseValue(),
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
