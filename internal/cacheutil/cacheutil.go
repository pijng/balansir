package cacheutil

import (
	"balansir/internal/helpers"
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	offset64        = 14695981039346656037
	prime64         = 1099511628211
	headerEntrySize = 4
	timeEntrySize   = 4
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
func New(shardsAmount int, maxSize int, exceedFallback bool, cacheAlgorithm string) *CacheCluster {
	cache := &CacheCluster{
		shards:         make([]*shard, shardsAmount),
		shardsAmount:   shardsAmount,
		exceedFallback: exceedFallback,
	}
	for i := 0; i < shardsAmount; i++ {
		cache.shards[i] = createShard(maxSize*mbBytes, cacheAlgorithm)
	}

	go func() {
		timer := time.NewTicker(1 * time.Second)
		for {
			select {
			case <-timer.C:
				cache.invalidate(time.Now().Unix())
			}
		}
	}()

	return cache
}

func (cluster *CacheCluster) getShard(hashedKey uint64) *shard {
	return cluster.shards[hashedKey&uint64(cluster.shardsAmount-1)]
}

//Set ...
func (cluster *CacheCluster) Set(key string, value []byte, TTL string) (err error) {
	hashedKey := cluster.hash.sum(key)
	shard := cluster.getShard(hashedKey)
	shard.mux.Lock()

	if len(value)+headerEntrySize > shard.maxSize {
		shard.mux.Unlock()
		return fmt.Errorf("value size is bigger than shard max size: %v out of %v", len(value)+headerEntrySize, shard.maxSize)
	}

	if shard.currentSize+len(value)+headerEntrySize >= shard.maxSize {
		shard.mux.Unlock()

		if shard.policy != nil {
			if err := shard.evict(len(value) + headerEntrySize); err != nil {
				return err
			}
			return nil
		}

		if cluster.exceedFallback {
			if err := setToFallbackShard(cluster.hash, cluster.shards, shard, hashedKey, value, TTL); err != nil {
				return err
			}
			return nil
		}

		return errors.New("potential exceeding of shard max capacity")
	}
	shard.mux.Unlock()
	shard.set(hashedKey, value, TTL)
	return nil
}

//Get ...
func (cluster *CacheCluster) Get(key string) ([]byte, error) {
	hashedKey := cluster.hash.sum(key)
	shard := cluster.getShard(hashedKey)
	value, itemIndex, err := shard.get(hashedKey)
	if cluster.exceedFallback {
		if strings.Contains(string(value), "shard_reference_") {
			hashedKey = cluster.hash.sum(string(value))
			splittedVal := strings.Split(string(value), "shard_reference_")
			index, _ := strconv.Atoi(strings.Split(splittedVal[1], "_val_")[0])
			shard = cluster.shards[index]
			value, itemIndex, err = shard.get(hashedKey)
		}
	}
	if err == nil && shard.policy != nil {
		shard.policy.updateMetaValue(itemIndex)
	}
	return value, err
}

func (cluster *CacheCluster) invalidate(timestamp int64) {
	for _, shard := range cluster.shards {
		shard.clean(timestamp)
	}
}

type shard struct {
	hashmap      map[uint64]uint32
	items        []byte
	tail         int
	mux          sync.RWMutex
	headerBuffer []byte
	timeBuffer   []byte
	maxSize      int
	currentSize  int
	policy       *Meta
}

func createShard(maxSize int, cacheAlgorithm string) *shard {
	s := &shard{
		hashmap:      make(map[uint64]uint32),
		items:        make([]byte, maxSize),
		tail:         0,
		headerBuffer: make([]byte, headerEntrySize),
		timeBuffer:   make([]byte, timeEntrySize),
		maxSize:      maxSize,
	}

	if cacheAlgorithm != "" {
		s.policy = NewMeta(cacheAlgorithm)
	}

	return s
}

func (s *shard) set(hashedKey uint64, value []byte, TTL string) {
	s.mux.Lock()
	index := s.push(value, TTL)
	s.hashmap[hashedKey] = uint32(index)
	if s.policy != nil {
		s.policy.push(uint32(index), hashedKey)
	}
	s.mux.Unlock()
}

func (s *shard) push(value []byte, TTL string) int {
	dataLen := len(value)
	index := s.tail
	duration := helpers.GetDuration(TTL)
	s.save(value, dataLen, index, duration)
	return index
}

func (s *shard) save(value []byte, length int, index int, duration time.Duration) {
	binary.LittleEndian.PutUint32(s.headerBuffer, uint32(length))
	binary.LittleEndian.PutUint32(s.timeBuffer, uint32(time.Now().Add(duration).Unix()))

	totalLen := headerEntrySize + timeEntrySize + length
	tmpBuffer := make([]byte, totalLen)

	copy(tmpBuffer[0:], s.headerBuffer)
	copy(tmpBuffer[headerEntrySize:], s.timeBuffer)
	copy(tmpBuffer[headerEntrySize+timeEntrySize:], value)

	copy(s.items[index:], tmpBuffer)

	s.tail += headerEntrySize + timeEntrySize + length
	s.currentSize += headerEntrySize + timeEntrySize + length
}

func (s *shard) get(hashedKey uint64) ([]byte, uint32, error) {
	s.mux.RLock()
	itemIndex, ok := s.hashmap[hashedKey]
	if !ok {
		s.mux.RUnlock()
		return nil, 0, errors.New("key not found")
	}
	blockSize := int(binary.LittleEndian.Uint32(s.items[itemIndex : itemIndex+headerEntrySize]))
	value := s.items[itemIndex+headerEntrySize+timeEntrySize : int(itemIndex)+headerEntrySize+timeEntrySize+blockSize]
	s.mux.RUnlock()
	return value, itemIndex, nil
}

func (s *shard) delete(keyIndex uint64, itemIndex uint32, valueSize int) {
	delete(s.hashmap, keyIndex)
	for k := 0; k < valueSize; k++ {
		s.items[int(itemIndex)+k] = 0
	}
	s.tail -= valueSize
	s.currentSize -= valueSize
}

func (s *shard) clean(timestamp int64) {
	s.mux.Lock()
	defer s.mux.Unlock()
	if len(s.hashmap) > 0 {
		for keyIndex, itemIndex := range s.hashmap {
			blockSize := int(binary.LittleEndian.Uint32(s.items[itemIndex : itemIndex+headerEntrySize]))
			timeValue := s.items[itemIndex+headerEntrySize : itemIndex+headerEntrySize+timeEntrySize]
			keepTill := binary.LittleEndian.Uint32(timeValue)
			if uint32(timestamp) > keepTill {
				s.delete(keyIndex, itemIndex, blockSize+headerEntrySize+timeEntrySize)
			}
		}
	}
}

func (s *shard) evict(valueSize int) error {
	itemIndex, keyIndex, err := s.policy.evict()
	if err != nil {
		return err
	}
	s.mux.Lock()
	s.delete(keyIndex, itemIndex, valueSize)

	if s.currentSize+valueSize >= s.maxSize {
		s.mux.Unlock()
		s.evict(valueSize)
	}

	s.mux.Unlock()
	return nil
}

func setToFallbackShard(hasher fnv64a, shards []*shard, exactShard *shard, hashedKey uint64, value []byte, TTL string) (err error) {
	for i, shard := range shards {
		shard.mux.Lock()
		if shard.currentSize+len(value)+headerEntrySize < shard.maxSize {
			shard.mux.Unlock()
			md := md5.Sum(value)
			valueHash := hex.EncodeToString(md[:16])
			ref := fmt.Sprintf("shard_reference_%v_val_%v", i, valueHash)
			shard.set(hasher.sum(ref), value, TTL)
			exactShard.set(hashedKey, []byte(ref), TTL)
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
	_, err := w.Write(bodyBuf.Bytes())
	if err != nil {
		log.Println(err)
	}
}

//GCPercentRatio ...
func GCPercentRatio(a int, s int) int {
	val, _ := strconv.Atoi(fmt.Sprintf("%.0f", 30*(100/(float64(a)*float64(s)))))
	return helpers.Max(val, 1)
}
