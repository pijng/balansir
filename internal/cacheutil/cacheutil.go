package cacheutil

import (
	"encoding/binary"
	"errors"
	"sync"
)

const (
	offset64        = 14695981039346656037
	prime64         = 1099511628211
	headerEntrySize = 4
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

type cacheCluster struct {
	shards       []*shard
	hash         fnv64a
	shardsAmount int
}

func New(shardsAmount int, maxSize int) *cacheCluster {
	cache := &cacheCluster{
		shards:       make([]*shard, shardsAmount),
		shardsAmount: shardsAmount,
	}
	for i := 0; i < shardsAmount; i++ {
		cache.shards[i] = createShard(maxSize)
	}

	return cache
}

func (cluster *cacheCluster) getShard(hashedKey uint64) *shard {
	return cluster.shards[hashedKey&uint64(cluster.shardsAmount-1)]
}

func (cluster *cacheCluster) set(key string, value []byte) {
	hashedKey := cluster.hash.sum(key)
	shard := cluster.getShard(hashedKey)
	shard.set(hashedKey, value)
}

func (cluster *cacheCluster) get(key string) ([]byte, error) {
	hashedKey := cluster.hash.sum(key)
	shard := cluster.getShard(hashedKey)
	value, _ := shard.get(key, hashedKey)
	return value, nil
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
	itemIndex := int(s.items[hashedKey])
	if itemIndex == 0 {
		s.mux.RUnlock()
		return nil, errors.New("key not found")
	}
	blockSize := int(binary.LittleEndian.Uint32(s.items[itemIndex : itemIndex+headerEntrySize]))
	entry := s.items[itemIndex+headerEntrySize : itemIndex+headerEntrySize+blockSize]
	s.mux.RUnlock()
	return readEntry(entry), nil
}
