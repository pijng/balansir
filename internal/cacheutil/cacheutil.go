package cacheutil

import (
	"balansir/internal/helpers"
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
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
	shards         []*shard
	hash           fnv64a
	shardsAmount   int
	exceedFallback bool
}

//New ...
func New(shardsAmount int, maxSize int, exceedFallback bool) *CacheCluster {
	cache := &CacheCluster{
		shards:         make([]*shard, shardsAmount),
		shardsAmount:   shardsAmount,
		exceedFallback: exceedFallback,
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
func (cluster *CacheCluster) Set(key string, value []byte) (err error) {
	hashedKey := cluster.hash.sum(key)
	shard := cluster.getShard(hashedKey)
	shard.mux.Lock()
	if shard.currentSize+len(value)+headerEntrySize >= shard.maxSize {
		shard.mux.Unlock()
		if cluster.exceedFallback {
			if err := setToFallbackShard(cluster.hash, cluster.shards, shard, hashedKey, value); err != nil {
				return err
			}
			return nil
		}

		return errors.New("potential exceeding of shard max capacity")
	}
	shard.mux.Unlock()
	shard.set(hashedKey, value)
	return nil
}

//Get ...
func (cluster *CacheCluster) Get(key string) ([]byte, error) {
	hashedKey := cluster.hash.sum(key)
	shard := cluster.getShard(hashedKey)
	value, err := shard.get(hashedKey)
	if cluster.exceedFallback {
		if strings.Contains(string(value), "shard_reference_") {
			hashedKey = cluster.hash.sum(string(value))
			splittedVal := strings.Split(string(value), "shard_reference_")
			index, _ := strconv.Atoi(strings.Split(splittedVal[1], "_val_")[0])
			shard = cluster.shards[index]
			value, err = shard.get(hashedKey)
		}
	}
	return value, err
}

type shard struct {
	hashmap      map[uint64]uint32
	items        []byte
	tail         int
	mux          sync.RWMutex
	headerBuffer []byte
	maxSize      int
	currentSize  int
}

func createShard(maxSize int) *shard {
	return &shard{
		hashmap:      make(map[uint64]uint32),
		items:        make([]byte, 0, maxSize),
		tail:         0,
		headerBuffer: make([]byte, headerEntrySize),
		maxSize:      maxSize,
	}
}

func (s *shard) set(hashedKey uint64, value []byte) {
	entry := wrapEntry(value)
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
	s.items = append(s.items, s.headerBuffer...)
	s.items = append(s.items, value...)
	s.tail += headerEntrySize + len
	s.currentSize += headerEntrySize + len
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

func (s *shard) get(hashedKey uint64) ([]byte, error) {
	s.mux.RLock()
	itemIndex, ok := s.hashmap[hashedKey]
	if !ok {
		s.mux.RUnlock()
		return nil, errors.New("key not found")
	}
	blockSize := int(binary.LittleEndian.Uint32(s.items[itemIndex : itemIndex+headerEntrySize]))
	entry := s.items[itemIndex+headerEntrySize : int(itemIndex)+headerEntrySize+blockSize]
	value := readEntry(entry)
	s.mux.RUnlock()
	return value, nil
}

func setToFallbackShard(hasher fnv64a, shards []*shard, exactShard *shard, hashedKey uint64, value []byte) (err error) {
	for i, shard := range shards {
		shard.mux.Lock()
		if shard.currentSize+len(value)+headerEntrySize < shard.maxSize {
			shard.mux.Unlock()
			md := md5.Sum(value)
			valueHash := hex.EncodeToString(md[:16])
			ref := fmt.Sprintf("shard_reference_%v_val_%v", i, valueHash)
			shard.set(hasher.sum(ref), value)
			exactShard.set(hashedKey, []byte(ref))
			return nil
		}
		shard.mux.Unlock()
	}
	return errors.New("potential exceeding of any shard max capacity")
}

//ServeFromCache ...
func ServeFromCache(w http.ResponseWriter, r *http.Request, response []byte) {
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

//GCPercentRatio ...
func GCPercentRatio(a int, s int) int {
	val, _ := strconv.Atoi(fmt.Sprintf("%.0f", 30*(100/(float64(a)*float64(s)))))
	return helpers.Max(val, 1)
}
