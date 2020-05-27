package cacheutil

import (
	"encoding/binary"
	"errors"
	"sync"
)

//KeyStorage ...
type KeyStorage struct {
	hashmap map[uint64]uint32
	keys    []byte
	tail    uint32
	mux     sync.RWMutex
}

//NewKeyStorage ...
func NewKeyStorage() *KeyStorage {
	return &KeyStorage{
		hashmap: make(map[uint64]uint32),
		keys:    make([]byte, 0),
		tail:    0,
	}
}

//SetHashedKey ...
func (ks *KeyStorage) SetHashedKey(key string, hashedKey uint64) {
	ks.mux.Lock()
	defer ks.mux.Unlock()
	index := ks.tail
	ks.hashmap[hashedKey] = index

	sizeBuffer := make([]byte, headerEntrySize)
	keyLen := len(key)
	binary.LittleEndian.PutUint32(sizeBuffer, uint32(keyLen))

	tmpBuffer := make([]byte, keyLen+headerEntrySize)
	copy(tmpBuffer[0:], sizeBuffer)
	copy(tmpBuffer[headerEntrySize:], []byte(key))

	ks.keys = append(ks.keys, tmpBuffer...)
	ks.tail += uint32(headerEntrySize + keyLen)
}

//GetInitialKey ...
func (ks *KeyStorage) GetInitialKey(hashedKey uint64) (string, error) {
	ks.mux.RLock()
	itemIndex, ok := ks.hashmap[hashedKey]
	if !ok {
		ks.mux.RUnlock()
		return "", errors.New("url key not found")
	}
	blockSize := int(binary.LittleEndian.Uint32(ks.keys[itemIndex : itemIndex+headerEntrySize]))
	value := string(ks.keys[itemIndex+headerEntrySize : int(itemIndex)+headerEntrySize+blockSize])
	ks.mux.RUnlock()
	return value, nil
}
