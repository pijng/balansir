package rateutil

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

//Rate ...
type Rate struct {
	Mux         sync.RWMutex
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
	rate.Mux.Lock()
	defer rate.Mux.Unlock()

	rate.ratemap = append(rate.ratemap[1:], make([]int64, 1)[0])
	rate.responsemap = append(rate.responsemap[1:], make([]int64, 1)[0])
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
	val := float64(atomic.LoadInt64(&rate.responsemap[0])) / math.Max(rate.RateValue(), 1) / 1000
	return math.Round(val*100) / 100
}
