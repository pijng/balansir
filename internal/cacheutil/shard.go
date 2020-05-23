package cacheutil

import (
	"balansir/internal/helpers"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

//Shard ...
type Shard struct {
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

//CreateShard ...
func CreateShard(maxSize int, cacheAlgorithm string) *Shard {
	s := &Shard{
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

func (s *Shard) set(hashedKey uint64, value []byte, TTL string) {
	s.mux.Lock()
	index := s.push(value, TTL)
	s.hashmap[hashedKey] = uint32(index)
	if s.policy != nil {
		s.policy.push(uint32(index), hashedKey)
	}
	s.mux.Unlock()
}

func (s *Shard) push(value []byte, TTL string) int {
	dataLen := len(value)
	index := s.tail
	duration := helpers.GetDuration(TTL)
	s.save(value, dataLen, index, duration)
	return index
}

func (s *Shard) save(value []byte, length int, index int, duration time.Duration) {
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

func (s *Shard) get(hashedKey uint64) ([]byte, uint32, error) {
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

func (s *Shard) delete(keyIndex uint64, itemIndex uint32, valueSize int) {
	delete(s.hashmap, keyIndex)
	for k := 0; k < valueSize; k++ {
		s.items[int(itemIndex)+k] = 0
	}
	s.tail -= valueSize
	s.currentSize -= valueSize
}

func (s *Shard) clean(timestamp int64) {
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

func (s *Shard) evict(valueSize int) error {
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

func setToFallbackShard(hasher fnv64a, shards []*Shard, exactShard *Shard, hashedKey uint64, value []byte, TTL string) (err error) {
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
