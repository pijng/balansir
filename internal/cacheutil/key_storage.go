package cacheutil

import (
	"errors"
	"sync"
)

//KeyStorage ...
type KeyStorage struct {
	hashmap map[uint64]string
	mux     sync.RWMutex
}

//NewKeyStorage ...
func NewKeyStorage() *KeyStorage {
	return &KeyStorage{
		hashmap: make(map[uint64]string),
	}
}

//SetHashedKey ...
func (ks *KeyStorage) SetHashedKey(key string, hashedKey uint64) {
	ks.mux.Lock()
	defer ks.mux.Unlock()

	ks.hashmap[hashedKey] = key
}

//GetInitialKey ...
func (ks *KeyStorage) GetInitialKey(hashedKey uint64) (string, error) {
	ks.mux.RLock()

	value, ok := ks.hashmap[hashedKey]
	if !ok {
		ks.mux.RUnlock()
		return "", errors.New("url key not found")
	}

	ks.mux.RUnlock()
	return value, nil
}
