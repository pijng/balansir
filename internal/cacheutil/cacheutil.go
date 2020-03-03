package cacheutil

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net/http"
	"sync"
)

const (
	offset64        = 14695981039346656037
	prime64         = 1099511628211
	headerEntrySize = 4
	mbBytes         = 1048576
)

type fnv64a struct{}

func (f fnv64a) sum(key string) uint64 {
	var hash uint64 = offset64
	for i := 0; i < len(key); i++ {
		hash ^= uint64(key[i])
		hash *= prime64
	}

	return hash
}

//CacheCluster ...
type CacheCluster struct {
	shards       []*shard
	hash         fnv64a
	shardsAmount int
}

//New ...
func New(shardsAmount int, maxSize int) *CacheCluster {
	cache := &CacheCluster{
		shards:       make([]*shard, shardsAmount),
		shardsAmount: shardsAmount,
	}
	for i := 0; i < shardsAmount; i++ {
		cache.shards[i] = createShard(maxSize * mbBytes)
	}

	return cache
}

func (cluster *CacheCluster) getShard(hashedKey uint64) *shard {
	return cluster.shards[hashedKey&uint64(cluster.shardsAmount-1)]
}

//Set ...
func (cluster *CacheCluster) Set(key string, value []byte) {
	hashedKey := cluster.hash.sum(key)
	shard := cluster.getShard(hashedKey)
	shard.set(hashedKey, value)
}

//Get ...
func (cluster *CacheCluster) Get(key string) ([]byte, error) {
	hashedKey := cluster.hash.sum(key)
	shard := cluster.getShard(hashedKey)
	value, err := shard.get(key, hashedKey)
	return value, err
}

type shard struct {
	hashmap      map[uint64]uint32
	items        []byte
	tail         int
	mux          sync.RWMutex
	headerBuffer []byte
}

func createShard(maxSize int) *shard {
	return &shard{
		hashmap:      make(map[uint64]uint32, maxSize),
		items:        make([]byte, maxSize),
		tail:         1,
		headerBuffer: make([]byte, headerEntrySize),
	}
}

func (s *shard) set(hashedKey uint64, value []byte) {
	entry := readEntry(value)
	s.mux.Lock()
	index := s.push(entry)
	s.hashmap[hashedKey] = uint32(index)
	s.mux.Unlock()
}

func (s *shard) push(value []byte) int {
	dataLen := len(value)
	index := s.tail
	s.save(value, dataLen)
	return index
}

func (s *shard) save(value []byte, len int) {
	binary.LittleEndian.PutUint32(s.headerBuffer, uint32(len))
	s.tail += copy(s.items[s.tail:], s.headerBuffer)
	s.tail += copy(s.items[s.tail:], value[:len])
}

func readEntry(value []byte) []byte {
	blob := make([]byte, len(value))
	copy(blob, value)
	return blob
}

func wrapEntry(value []byte) []byte {
	// here I can put timestamps and stuff for cache invalidation
	blob := make([]byte, len(value))
	copy(blob, value)
	return blob
}

func (s *shard) get(key string, hashedKey uint64) ([]byte, error) {
	s.mux.RLock()
	itemIndex := int(s.hashmap[hashedKey])
	if itemIndex == 0 {
		s.mux.RUnlock()
		return nil, errors.New("key not found")
	}
	blockSize := int(binary.LittleEndian.Uint32(s.items[itemIndex : itemIndex+headerEntrySize]))
	entry := s.items[itemIndex+headerEntrySize : itemIndex+headerEntrySize+blockSize]
	s.mux.RUnlock()
	return readEntry(entry), nil
}

//ServeFromCache ...
func ServeFromCache(w http.ResponseWriter, r *http.Request, cacheCluster *CacheCluster, response []byte) {
	//First we need to split headers from our cached response and assign it to responseWriter
	slicedResponse := bytes.Split(response, []byte(";--;"))
	//Iterate over sliced headers
	for _, val := range slicedResponse {
		//Split `key`–`value` parts and iterate over 'em
		slicedHeader := bytes.Split(val, []byte(";-;"))
		for i := range slicedHeader {
			//Guard to prevent writing last header value as new header key
			if i+1 <= len(slicedHeader)-1 {
				//Write header `key`-`value` to responseWriter
				w.Header().Set(string(slicedHeader[i]), string(slicedHeader[i+1]))
			}
		}
	}
	//Create new buffer for our cached response
	bodyBuf := bytes.NewBuffer([]byte{})
	//Write body to buffer. It'll always be the last element of our slice
	bodyBuf.Write(slicedResponse[len(slicedResponse)-1])
	//Write response buffer to responseWriter and return it to client
	w.Write(bodyBuf.Bytes())
	return
}
