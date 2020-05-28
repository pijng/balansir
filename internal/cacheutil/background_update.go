package cacheutil

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

//Updater ...
type Updater struct {
	client     *http.Client
	keyStorage *KeyStorage
	port       int
}

//NewUpdater ...
func NewUpdater(port int, transportTimeout int, dialerTimeout int) *Updater {
	return &Updater{
		client: &http.Client{
			Timeout: time.Duration(transportTimeout) * time.Second,
			Transport: &http.Transport{
				Dial: (&net.Dialer{
					Timeout: time.Duration(dialerTimeout) * time.Second,
				}).Dial,
			},
		},
		keyStorage: NewKeyStorage(),
		port:       port,
	}
}

//InvalidateCachedResponse ...
func (u *Updater) InvalidateCachedResponse(url string, mux *sync.RWMutex) error {
	mux.Unlock()
	defer mux.Lock()

	_, err := u.client.Get(fmt.Sprintf("http://127.0.0.1:%v%v", u.port, url))
	if err != nil {
		return err
	}
	return nil
}
