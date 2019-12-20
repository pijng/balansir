package ratelimit

import (
	"balansir/internal/confg"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

//Limiter ...
type Limiter map[string]*visitor

//GetVisitor ...
func (v *Limiter) GetVisitor(ip string, mu *sync.Mutex, configuration *confg.Configuration) *rate.Limiter {
	mu.Lock()
	defer mu.Unlock()

	if (*v) == nil {
		(*v) = make(Limiter)
	}
	limiter, exists := (*v)[ip]
	if !exists {
		limiter := rate.NewLimiter(rate.Limit(configuration.RatePerSecond), configuration.RateBucket)
		(*v)[ip] = &visitor{limiter, time.Now()}
		return limiter
	}
	limiter.lastSeen = time.Now()
	return limiter.limiter
}

//CleanOldVisitors ...
func (v *Limiter) CleanOldVisitors(mu *sync.Mutex) {
	for {
		mu.Lock()

		for ip, val := range *v {
			if time.Now().Sub(val.lastSeen) > 3*time.Minute {
				delete(*v, ip)
			}
		}
		mu.Unlock()
		time.Sleep(time.Minute)
	}
}
