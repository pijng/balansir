package rateutil

import (
	"expvar"
	"sync"
	"time"
)

//Rate ...
type Rate struct {
	mux         sync.RWMutex
	ratemap     []expvar.Int
	responsemap []expvar.Float
}

//NewRateCounter ...
func NewRateCounter() *Rate {
	rate := &Rate{
		ratemap:     make([]expvar.Int, 2),
		responsemap: make([]expvar.Float, 2),
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
	rate.mux.Lock()
	defer rate.mux.Unlock()

	rate.ratemap = append(rate.ratemap[1:], make([]expvar.Int, 1)[0])
	rate.responsemap = append(rate.responsemap[1:], make([]expvar.Float, 1)[0])
}

//RateIncrement ...
func (rate *Rate) RateIncrement() {
	rate.ratemap[1].Add(1)
}

//RateValue ...
func (rate *Rate) RateValue() int64 {
	return rate.ratemap[0].Value()
}

//ResponseCount ...
func (rate *Rate) ResponseCount(rt time.Time) {
	responseTime := time.Since(rt)
	rate.responsemap[1].Add(float64(responseTime.Milliseconds()))
}

//ResponseValue ...
func (rate *Rate) ResponseValue() float64 {
	if rate.responsemap[0].Value() != 0 {
		return float64(rate.RateValue()) / rate.responsemap[0].Value()
	}
	return 0
}
