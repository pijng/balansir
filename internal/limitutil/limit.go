package limitutil

import (
	"balansir/internal/configutil"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

//Limiter ...
type Limiter struct {
	mux  sync.RWMutex
	list map[string]*visitor
}

var limiter *Limiter
var once sync.Once

//GetLimiter ...
func GetLimiter() *Limiter {
	once.Do(func() {
		limiter = &Limiter{
			list: make(map[string]*visitor),
		}
	})

	return limiter
}

//GetVisitor ...
func (v *Limiter) GetVisitor(ip string, configuration *configutil.Configuration) *rate.Limiter {
	v.mux.Lock()
	defer v.mux.Unlock()

	limiter, exists := v.list[ip]
	if !exists {
		limiter := rate.NewLimiter(rate.Limit(configuration.RatePerSecond), configuration.RateBucket)
		v.list[ip] = &visitor{
			limiter:  limiter,
			lastSeen: time.Now(),
		}
		return limiter
	}
	limiter.lastSeen = time.Now()
	return limiter.limiter
}

//CleanOldVisitors ...
func (v *Limiter) CleanOldVisitors() {
	ticker := time.NewTicker(1 * time.Second)
	for {
		<-ticker.C

		v.mux.Lock()
		for ip, val := range v.list {
			if time.Since(val.lastSeen) > 1*time.Second {
				delete(v.list, ip)
			}
		}
		v.mux.Unlock()
	}
}
