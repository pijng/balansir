package rateutil

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

//Rate ...
type Rate struct {
	mux         sync.RWMutex
	ratemap     []int64
	responsemap []int64
}

//NewRateCounter ...
func NewRateCounter() *Rate {
	rate := &Rate{
		ratemap:     make([]int64, 2),
		responsemap: make([]int64, 2),
	}

	go func() {
		timer := time.NewTicker(1 * time.Second)
		for {
			select {
			case <-timer.C:
				rate.swapMap()
			}
		}
	}()

	return rate
}

func (rate *Rate) swapMap() {
	// The last second's stats are accumulated within second index of the corresponding map,
	// so that we must "swap" value indexes when the current second ends, to make it available
	// for the dashboard, because value at the first index is returned to the `metricsutil.Stats`
	for _, accumulator := range [][]int64{rate.ratemap, rate.responsemap} {
		atomic.StoreInt64(&accumulator[0], atomic.LoadInt64(&accumulator[1]))
		atomic.StoreInt64(&accumulator[1], 0)
	}
}

//RateIncrement ...
func (rate *Rate) RateIncrement() {
	atomic.AddInt64(&rate.ratemap[1], 1)
}

//RateValue ...
func (rate *Rate) RateValue() float64 {
	return float64(atomic.LoadInt64(&rate.ratemap[0]))
}

//ResponseCount ...
func (rate *Rate) ResponseCount(rt time.Time) {
	responseTime := time.Since(rt)
	atomic.AddInt64(&rate.responsemap[1], responseTime.Microseconds())
}

//ResponseValue ...
func (rate *Rate) ResponseValue() float64 {
	if rate.RateValue() > 0 {
		val := float64(atomic.LoadInt64(&rate.responsemap[0])) / rate.RateValue() / 1000
		return math.Round(val*100) / 100
	}
	return 0
}
