package statusutil

import "sync"

//StatusCodes ...
type StatusCodes struct {
	Storage map[int]int64
	mux     sync.RWMutex
}

var statusCodes *StatusCodes
var once sync.Once

//GetStatusCodes ...
func GetStatusCodes() *StatusCodes {
	once.Do(func() {
		statusCodes = &StatusCodes{
			Storage: make(map[int]int64),
		}
	})

	return statusCodes
}

//HitStatus ...
func (sm *StatusCodes) HitStatus(code int) {
	sm.mux.RLock()
	defer sm.mux.RUnlock()

	sm.Storage[code]++
}
