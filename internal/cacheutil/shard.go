package cacheutil

import (
	"balansir/internal/logutil"
	"errors"
	"sync"
	"time"
)

//Shard ...
type Shard struct {
	Hashmap     map[uint64]shardItem
	Items       map[int][]byte
	Tail        int
	mux         sync.RWMutex
	priorMux    sync.RWMutex
	size        int
	CurrentSize int
	Policy      *Meta
}

type shardItem struct {
	Index  int
	Length int
	TTL    int64
}

//CreateShard ...
func CreateShard(size int, CachePolicy string) *Shard {
	s := &Shard{
		Hashmap:     make(map[uint64]shardItem),
		Items:       make(map[int][]byte),
		Tail:        0,
		size:        size,
		CurrentSize: 0,
		Policy:      NewMeta(CachePolicy),
	}

	return s
}

func (s *Shard) set(hashedKey uint64, value []byte, TTL string) {
	index := s.push(value)
	duration := getDuration(TTL)
	ttl := time.Now().Add(duration).Unix()

	s.Hashmap[hashedKey] = shardItem{Index: index, Length: len(value), TTL: ttl}
	s.Policy.push(index, hashedKey, TTL)
}

func (s *Shard) push(value []byte) int {
	dataLen := len(value)
	index := s.Tail
	s.save(value, dataLen, index)
	return index
}

func (s *Shard) save(value []byte, valueSize int, index int) {
	s.Items[index] = value

	s.Tail++
	s.CurrentSize += valueSize
}

func (s *Shard) get(hashedKey uint64) ([]byte, error) {
	s.mux.RLock()
	defer s.mux.RUnlock()

	item, ok := s.Hashmap[hashedKey]
	if !ok {
		return nil, errors.New("key not found")
	}

	value := s.Items[item.Index]
	return value, nil
}

func (s *Shard) delete(keyIndex uint64, itemIndex int, valueSize int) {
	delete(s.Hashmap, keyIndex)
	delete(s.Items, itemIndex)

	s.Policy.mux.Lock()
	delete(s.Policy.HashMap, keyIndex)
	s.Policy.mux.Unlock()

	s.CurrentSize -= valueSize
}

func (s *Shard) update(timestamp int64, updater *Updater) {
	s.mux.Lock()
	defer s.mux.Unlock()
	if len(s.Hashmap) <= 0 {
		return
	}

	for keyIndex := range s.Hashmap {
		ttl := s.Hashmap[keyIndex].TTL

		if s.Policy.TimeBased() {
			ttl = s.Policy.HashMap[keyIndex].Value
		}

		if timestamp <= ttl {
			return
		}

		s.delete(keyIndex, s.Hashmap[keyIndex].Index, s.Hashmap[keyIndex].Length)

		cluster := GetCluster()
		cluster.backupManager.Hit()

		if cluster.backgroundUpdate && updater != nil {
			urlString, err := updater.keyStorage.GetInitialKey(keyIndex)
			if err != nil {
				logutil.Warning(err)
				continue
			}

			err = updater.InvalidateCachedResponse(urlString, &s.mux)
			if err != nil {
				logutil.Error(err)
			}
		}
	}
}

func (s *Shard) retryEvict(pendingValueSize int) error {
	itemIndex, keyIndex, err := s.Policy.evict()
	if err != nil {
		return err
	}

	s.delete(keyIndex, itemIndex, s.Hashmap[keyIndex].Length)

	if s.size-s.CurrentSize <= pendingValueSize {
		if err := s.retryEvict(pendingValueSize); err != nil {
			logutil.Warning(err)
		}
	}

	return nil
}

func (s *Shard) evict(pendingValueSize int) error {
	itemIndex, keyIndex, err := s.Policy.evict()
	if err != nil {
		return err
	}

	s.delete(keyIndex, itemIndex, s.Hashmap[keyIndex].Length)

	if s.size-s.CurrentSize <= pendingValueSize {
		if err := s.retryEvict(pendingValueSize); err != nil {
			logutil.Warning(err)
		}
	}

	return nil
}
