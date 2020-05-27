package cacheutil

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
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
func (u *Updater) InvalidateCachedResponse(url string, mux *sync.RWMutex) ([]byte, error) {
	mux.Unlock()
	defer mux.Lock()

	r, err := u.client.Get(fmt.Sprintf("http://127.0.0.1:%v%v", u.port, url))
	if err != nil {
		return nil, err
	}

	defer r.Body.Close()

	var headers []Header

	for key, val := range r.Header {
		header := Header{
			Key:   key,
			Value: val,
		}
		headers = append(headers, header)
	}

	body, _ := ioutil.ReadAll(r.Body)

	response := Response{
		Headers: headers,
		Body:    body,
	}

	resp, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}

	return resp, nil
}
