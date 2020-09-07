package cacheutil

import (
	"balansir/internal/logutil"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

//Shard ...
type Shard struct {
	Hashmap     map[uint64]shardItem
	Items       map[int][]byte
	Tail        int
	mux         sync.RWMutex
	size        int
	CurrentSize int
	policy      *Meta
}

type shardItem struct {
	Index  int
	Length int
	TTL    int64
}

//CreateShard ...
func CreateShard(size int, cacheAlgorithm string) *Shard {
	s := &Shard{
		Hashmap:     make(map[uint64]shardItem),
		Items:       make(map[int][]byte),
		Tail:        0,
		size:        size,
		CurrentSize: 0,
	}

	if cacheAlgorithm != "" {
		s.policy = NewMeta(cacheAlgorithm)
	}

	return s
}

func (s *Shard) set(hashedKey uint64, value []byte, TTL string) {
	s.mux.Lock()
	index := s.push(value)
	duration := getDuration(TTL)
	ttl := time.Now().Add(duration).Unix()
	s.Hashmap[hashedKey] = shardItem{Index: index, Length: len(value), TTL: ttl}

	if s.policy != nil {
		s.policy.push(index, hashedKey, TTL)
	}
	s.mux.Unlock()
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
	item, ok := s.Hashmap[hashedKey]
	if !ok {
		s.mux.RUnlock()
		return nil, errors.New("key not found")
	}
	value := s.Items[item.Index]
	s.mux.RUnlock()
	return value, nil
}

func (s *Shard) delete(keyIndex uint64, itemIndex int, valueSize int) {
	delete(s.Hashmap, keyIndex)
	delete(s.Items, itemIndex)

	if s.policy != nil {
		s.policy.mux.Lock()
		delete(s.policy.hashMap, keyIndex)
		s.policy.mux.Unlock()
	}

	s.CurrentSize -= valueSize
}

func (s *Shard) update(timestamp int64, updater *Updater) {
	s.mux.Lock()
	defer s.mux.Unlock()
	if len(s.Hashmap) > 0 {
		for keyIndex, item := range s.Hashmap {
			ttl := item.TTL

			if s.policy != nil {
				if s.policy.TimeBased() {
					ttl = s.policy.hashMap[keyIndex].value
				}
			}

			if timestamp > ttl {
				//delete stale version in any case
				s.delete(keyIndex, item.Index, item.Length)

				if updater != nil {
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
	}
}

func (s *Shard) retryEvict(pendingValueSize int) error {
	itemIndex, keyIndex, err := s.policy.evict()
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
	itemIndex, keyIndex, err := s.policy.evict()
	if err != nil {
		return err
	}
	s.mux.Lock()

	s.delete(keyIndex, itemIndex, s.Hashmap[keyIndex].Length)

	if s.size-s.CurrentSize <= pendingValueSize {
		if err := s.retryEvict(pendingValueSize); err != nil {
			logutil.Warning(err)
		}
	}

	s.mux.Unlock()
	return nil
}

func setToFallbackShard(hasher fnv64a, shards []*Shard, exactShard *Shard, hashedKey uint64, value []byte, TTL string) (err error) {
	for i, shard := range shards {
		shard.mux.Lock()
		if shard.CurrentSize+len(value) < shard.size {
			shard.mux.Unlock()
			md := md5.Sum(value)
			valueHash := hex.EncodeToString(md[:16])
			ref := fmt.Sprintf("shard_reference_%v_val_%v", i, valueHash)
			shard.set(hasher.Sum(ref), value, TTL)
			exactShard.set(hashedKey, []byte(ref), TTL)
			return nil
		}
		shard.mux.Unlock()
	}
	return errors.New("potential exceeding of any shard max capacity")
}
