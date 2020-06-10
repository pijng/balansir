package cacheutil

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

type ctxKey string

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

	var key ctxKey
	key = "background-update"
	req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%v%v", u.port, url), nil)
	ctx := context.WithValue(req.Context(), key, true)
	req = req.WithContext(ctx)
	_, err := u.client.Do(req)
	if err != nil {
		return err
	}
	return nil
}
