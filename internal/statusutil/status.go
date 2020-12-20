package statusutil

import (
	"sync"
)

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

//GetStatuses ...
func (sm *StatusCodes) GetStatuses() map[int]int64 {
	sm.mux.RLock()
	defer sm.mux.RUnlock()

	return sm.Storage
}

//HitStatus ...
func (sm *StatusCodes) HitStatus(code int) {
	sm.mux.Lock()
	defer sm.mux.Unlock()

	sm.Storage[code]++
}
