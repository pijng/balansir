package ratelimit

import (
	"balansir/internal/confg"
	"sync"

	"golang.org/x/time/rate"
)

type list map[string]*rate.Limiter

//Visitors ...
type Visitors struct {
	List list
}

//GetVisitor ...
func (v *Visitors) GetVisitor(ip string, mu *sync.Mutex, configuration *confg.Configuration) *rate.Limiter {
	mu.Lock()
	defer mu.Unlock()

	if v.List == nil {
		v.List = make(list)
	}
	limiter, exists := v.List[ip]
	if !exists {
		limiter = rate.NewLimiter(rate.Limit(configuration.RatePerSecond), configuration.RateBucket)
		v.List[ip] = limiter
	}
	return limiter
}
