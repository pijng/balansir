package cacheutil

import (
	"balansir/internal/helpers"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
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
	shards         []*Shard
	hash           fnv64a
	shardsAmount   int
	exceedFallback bool
}

//New ...
func New(shardsAmount int, maxSize int, exceedFallback bool, cacheAlgorithm string) *CacheCluster {
	cache := &CacheCluster{
		shards:         make([]*Shard, shardsAmount),
		shardsAmount:   shardsAmount,
		exceedFallback: exceedFallback,
	}
	for i := 0; i < shardsAmount; i++ {
		cache.shards[i] = CreateShard(maxSize*mbBytes, cacheAlgorithm)
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

func (cluster *CacheCluster) getShard(hashedKey uint64) *Shard {
	return cluster.shards[hashedKey&uint64(cluster.shardsAmount-1)]
}

//Set ...
func (cluster *CacheCluster) Set(key string, value []byte, TTL string) (err error) {
	hashedKey := cluster.hash.sum(key)
	shard := cluster.getShard(hashedKey)
	shard.mux.Lock()

	if len(value)+headerEntrySize+timeEntrySize > shard.maxSize {
		shard.mux.Unlock()
		return fmt.Errorf("value size is bigger than shard max size: %vmb out of %vmb", fmt.Sprintf("%.2f", float64(len(value))/1024/1024), shard.maxSize/1024/1024)
	}

	if shard.currentSize+len(value)+headerEntrySize+timeEntrySize >= shard.maxSize {
		shard.mux.Unlock()

		if shard.policy != nil {
			if err := shard.evict(len(value) + headerEntrySize + timeEntrySize); err != nil {
				return err
			}
			shard.set(hashedKey, value, TTL)
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

//ServeFromCache ...
func ServeFromCache(w http.ResponseWriter, r *http.Request, value []byte) {
	var response Response
	json.Unmarshal(value, &response)

	for _, header := range response.Headers {
		w.Header().Set(header.Key, strings.Join(header.Value, " "))
	}

	_, err := w.Write(response.Body)
	if err != nil {
		log.Println(err)
	}
}

//GCPercentRatio ...
func GCPercentRatio(a int, s int) int {
	val, _ := strconv.Atoi(fmt.Sprintf("%.0f", 30*(100/(float64(a)*float64(s)))))
	return helpers.Max(val, 1)
}
