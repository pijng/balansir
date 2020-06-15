package cacheutil

import (
	"balansir/internal/configutil"
	"balansir/internal/logutil"
	"bytes"
	"errors"
	"fmt"
	"math"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync/atomic"
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

func (f fnv64a) Sum(key string) uint64 {
	var hash uint64 = offset64
	for i := 0; i < len(key); i++ {
		hash ^= uint64(key[i])
		hash *= prime64
	}

	return hash
}

//CacheCluster ...
type CacheCluster struct {
	shards           []*Shard
	Hash             fnv64a
	ShardsAmount     int
	ShardMaxSize     int
	Queue            *Queue
	Hits             int64
	Misses           int64
	exceedFallback   bool
	backgroundUpdate bool
	updater          *Updater
	cacheRules       []*configutil.Rule
}

//CacheClusterArgs ...
type CacheClusterArgs struct {
	ShardsAmount     int
	MaxSize          int
	ExceedFallback   bool
	CacheAlgorithm   string
	BackgroundUpdate bool
	CacheRules       []*configutil.Rule
	TransportTimeout int
	DialerTimeout    int
	Port             int
}

//New ...
func New(args CacheClusterArgs) *CacheCluster {
	cache := &CacheCluster{
		shards:         make([]*Shard, args.ShardsAmount),
		ShardsAmount:   args.ShardsAmount,
		ShardMaxSize:   args.MaxSize,
		Queue:          NewQueue(),
		exceedFallback: args.ExceedFallback,
		cacheRules:     args.CacheRules,
	}

	if args.BackgroundUpdate {
		cache.backgroundUpdate = args.BackgroundUpdate
		cache.updater = NewUpdater(args.Port, args.TransportTimeout, args.DialerTimeout)
	}

	for i := 0; i < args.ShardsAmount; i++ {
		cache.shards[i] = CreateShard(args.MaxSize*mbBytes, args.CacheAlgorithm)
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
	return cluster.shards[hashedKey&uint64(cluster.ShardsAmount-1)]
}

//Set ...
func (cluster *CacheCluster) Set(key string, value []byte, TTL string) (err error) {
	hashedKey := cluster.Hash.Sum(key)
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

			if cluster.updater != nil {
				cluster.updater.keyStorage.SetHashedKey(key, hashedKey)
			}

			return nil
		}

		if cluster.exceedFallback {
			if err := setToFallbackShard(cluster.Hash, cluster.shards, shard, hashedKey, value, TTL); err != nil {
				return err
			}

			if cluster.updater != nil {
				cluster.updater.keyStorage.SetHashedKey(key, hashedKey)
			}

			return nil
		}

		return errors.New("potential exceeding of shard max capacity")
	}
	shard.mux.Unlock()
	shard.set(hashedKey, value, TTL)

	if cluster.updater != nil {
		cluster.updater.keyStorage.SetHashedKey(key, hashedKey)
	}

	return nil
}

//Get ...
func (cluster *CacheCluster) Get(key string, trackMisses bool) ([]byte, error) {
	hashedKey := cluster.Hash.Sum(key)
	shard := cluster.getShard(hashedKey)
	value, err := shard.get(hashedKey)
	if cluster.exceedFallback {
		if strings.Contains(string(value), "shard_reference_") {
			hashedKey = cluster.Hash.Sum(string(value))
			splittedVal := strings.Split(string(value), "shard_reference_")
			index, _ := strconv.Atoi(strings.Split(splittedVal[1], "_val_")[0])
			shard = cluster.shards[index]
			value, err = shard.get(hashedKey)
		}
	}
	if err == nil {
		atomic.AddInt64(&cluster.Hits, 1)
		if shard.policy != nil {
			shard.policy.updateMetaValue(hashedKey)
		}
	}
	if err != nil {
		if trackMisses {
			atomic.AddInt64(&cluster.Misses, 1)
		}
	}
	return value, err
}

func (cluster *CacheCluster) invalidate(timestamp int64) {
	for _, shard := range cluster.shards {
		shard.update(timestamp, cluster.updater)
	}
}

//ServeFromCache ...
func ServeFromCache(w http.ResponseWriter, r *http.Request, value []byte) {
	//First we need to split headers from our cached response and assign it to responseWriter
	slicedResponse := bytes.Split(value, []byte(";--;"))
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
		logutil.Error(err)
	}
}

//GetHitRatio ...
func (cluster *CacheCluster) GetHitRatio() float64 {
	hits := float64(atomic.LoadInt64(&cluster.Hits))
	misses := float64(atomic.LoadInt64(&cluster.Misses))
	return (hits / math.Max(hits+misses, 1)) * 100
}

//RedefineCache ...
func RedefineCache(args *CacheClusterArgs, cluster *CacheCluster) (*CacheCluster, error) {
	if cluster == nil {
		cacheCluster := New(*args)
		debug.SetGCPercent(GCPercentRatio(args.ShardsAmount, args.MaxSize))
		logutil.Info("Cache enabled")
		return cacheCluster, nil
	}

	newCluster := &CacheCluster{
		Hits:             cluster.Hits,
		Misses:           cluster.Misses,
		exceedFallback:   args.ExceedFallback,
		backgroundUpdate: args.BackgroundUpdate,
		cacheRules:       args.CacheRules,
		Queue:            cluster.Queue,
	}

	if args.BackgroundUpdate {
		if cluster.updater == nil {
			newCluster.updater = NewUpdater(args.Port, args.TransportTimeout, args.DialerTimeout)
		} else {
			newCluster.updater = cluster.updater
		}
	}

	//increase shards amount
	if cluster.ShardsAmount < args.ShardsAmount {
		for i := 0; i < args.ShardsAmount-cluster.ShardsAmount; i++ {
			shard := CreateShard(args.MaxSize*mbBytes, args.CacheAlgorithm)
			newCluster.shards = append(newCluster.shards, shard)
		}
		newCluster.shards = append(newCluster.shards, cluster.shards...)

		//reduce shards amount
	} else if cluster.ShardsAmount > args.ShardsAmount {
		var deletedShards int
		var emptyShards []*Shard
		var nonEmptyShards int
		diff := cluster.ShardsAmount - args.ShardsAmount

		for _, shard := range cluster.shards {
			if deletedShards != diff {
				if shard.currentSize > 0 {
					nonEmptyShards++
					continue
				}
				emptyShards = append(emptyShards, shard)
				deletedShards++
			}
		}

		if deletedShards < diff {
			return nil, fmt.Errorf("cannot delete %v shard(s), because shards amount is %v and there are %v non-empty shard(s)", diff, cluster.ShardsAmount, nonEmptyShards)
		}

		for _, shard := range cluster.shards {
			if !include(emptyShards, shard) {
				newCluster.shards = append(newCluster.shards, shard)
			}
		}
	} else {
		newCluster.shards = cluster.shards
		for i, shard := range newCluster.shards {
			if shard.currentSize/mbBytes > args.MaxSize {
				return nil, fmt.Errorf("shards capacity cannot be reduced to %vmb, because one of the shard's current size is %vmb", args.MaxSize, shard.currentSize/mbBytes)
			}
			newCluster.shards[i].maxSize = args.MaxSize * mbBytes
		}
	}

	newCluster.ShardMaxSize = args.MaxSize
	newCluster.ShardsAmount = len(newCluster.shards)

	debug.SetGCPercent(GCPercentRatio(args.ShardsAmount, args.MaxSize))
	return newCluster, nil
}

func include(list []*Shard, s *Shard) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

//GCPercentRatio ...
func GCPercentRatio(a int, s int) int {
	val, _ := strconv.Atoi(fmt.Sprintf("%.0f", 30*(100/(float64(a)*float64(s)))))
	return max(val, 1)
}

func max(x int, y int) int {
	if x > 100 {
		return 100
	}
	if x < y {
		return y
	}
	return x
}
