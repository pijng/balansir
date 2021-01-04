package rateutil

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

//Rate ...
type Rate struct {
	requestsCount   []int64
	responseTimeSum []int64
}

var rate *Rate
var once sync.Once

//GetRateCounter ...
func GetRateCounter() *Rate {
	once.Do(func() {
		rate = &Rate{
			requestsCount:   make([]int64, 2),
			responseTimeSum: make([]int64, 2),
		}

		go func() {
			timer := time.NewTicker(1 * time.Second)
			for {
				<-timer.C
				rate.swap()
			}
		}()
	})

	return rate
}

func (rate *Rate) swap() {
	atomic.StoreInt64(&rate.requestsCount[0], atomic.LoadInt64(&rate.requestsCount[1]))
	atomic.StoreInt64(&rate.requestsCount[1], 0)

	atomic.StoreInt64(&rate.responseTimeSum[0], atomic.LoadInt64(&rate.responseTimeSum[1]))
	atomic.StoreInt64(&rate.responseTimeSum[1], 0)
}

//HitRequest ...
func (rate *Rate) HitRequest() {
	atomic.AddInt64(&rate.requestsCount[1], 1)
}

//RequestsPerSecond ...
func (rate *Rate) RequestsPerSecond() float64 {
	return float64(atomic.LoadInt64(&rate.requestsCount[0]))
}

//CommitResponseTime ...
func (rate *Rate) CommitResponseTime(rt time.Time) {
	responseTime := time.Since(rt)
	atomic.AddInt64(&rate.responseTimeSum[1], responseTime.Microseconds())
}

//AverageResponseTime ...
func (rate *Rate) AverageResponseTime() float64 {
	if rate.RequestsPerSecond() > 0 {
		val := float64(atomic.LoadInt64(&rate.responseTimeSum[0])) / rate.RequestsPerSecond() / 1000
		return math.Round(val*100) / 100
	}
	return 0
}
