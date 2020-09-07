package cacheutil

import (
	"balansir/internal/configutil"
	"balansir/internal/logutil"
	"bytes"
	"crypto/md5"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const (
	offset64 = 14695981039346656037
	prime64  = 1099511628211
	mbBytes  = 1048576
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
	encoder          *gob.Encoder
	backup           *os.File
	shards           []*Shard
	Hash             fnv64a
	ShardsAmount     int
	ShardSize        int
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
	ShardSize        int
	ExceedFallback   bool
	CacheAlgorithm   string
	BackgroundUpdate bool
	CacheRules       []*configutil.Rule
	TransportTimeout int
	DialerTimeout    int
	Port             int
}

var cluster *CacheCluster

//New ...
func New(args CacheClusterArgs) *CacheCluster {
	cluster = &CacheCluster{
		shards:         make([]*Shard, args.ShardsAmount),
		ShardsAmount:   args.ShardsAmount,
		ShardSize:      args.ShardSize,
		Queue:          NewQueue(),
		exceedFallback: args.ExceedFallback,
		cacheRules:     args.CacheRules,
	}

	if args.BackgroundUpdate {
		cluster.backgroundUpdate = args.BackgroundUpdate
		cluster.updater = NewUpdater(args.Port, args.TransportTimeout, args.DialerTimeout)
	}

	for i := 0; i < args.ShardsAmount; i++ {
		cluster.shards[i] = CreateShard(args.ShardSize*mbBytes, args.CacheAlgorithm)
	}

	go RestoreCache(cluster)

	go func() {
		timer := time.NewTicker(1 * time.Second)
		for {
			select {
			case <-timer.C:
				cluster.invalidate(time.Now().Unix())
			}
		}
	}()

	return cluster
}

//GetCluster ...
func GetCluster() *CacheCluster {
	return cluster
}

func (cluster *CacheCluster) getShard(hashedKey uint64) *Shard {
	return cluster.shards[hashedKey&uint64(cluster.ShardsAmount-1)]
}

//Set ...
func (cluster *CacheCluster) Set(key string, value []byte, TTL string) (err error) {
	hashedKey := cluster.Hash.Sum(key)
	shard := cluster.getShard(hashedKey)
	shard.mux.Lock()

	if len(value) > shard.size {
		shard.mux.Unlock()
		return fmt.Errorf("value size is bigger than shard max size: %vmb out of %vmb", fmt.Sprintf("%.2f", float64(len(value))/1024/1024), shard.size/1024/1024)
	}

	if shard.currentSize+len(value) >= shard.size {
		shard.mux.Unlock()

		if shard.policy != nil {
			if err := shard.evict(len(value)); err != nil {
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

	go BackupCache(cluster)

	return nil
}

//Get ...
func (cluster *CacheCluster) Get(key string, trackMisses bool) ([]byte, error) {
	hashedKey := cluster.Hash.Sum(key)
	shard := cluster.getShard(hashedKey)
	value, err := shard.get(hashedKey)
	if cluster.exceedFallback {
		if bytes.Contains(value, []byte("shard_reference_")) {
			strValue := string(value)
			hashedKey = cluster.Hash.Sum(strValue)
			splittedVal := strings.Split(strValue, "shard_reference_")
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
		//Split `key`–`value` parts and iterate over them
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
func RedefineCache(args *CacheClusterArgs) error {
	if cluster == nil {
		cacheCluster := New(*args)
		debug.SetGCPercent(GCPercentRatio(args.ShardsAmount, args.ShardSize))
		logutil.Notice("Cache enabled")
		cluster = cacheCluster
		return nil
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
			shard := CreateShard(args.ShardSize*mbBytes, args.CacheAlgorithm)
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
			return fmt.Errorf("cannot delete %v shard(s), because shards amount is %v and there are %v non-empty shard(s)", diff, cluster.ShardsAmount, nonEmptyShards)
		}

		for _, shard := range cluster.shards {
			if !include(emptyShards, shard) {
				newCluster.shards = append(newCluster.shards, shard)
			}
		}
	} else {
		newCluster.shards = cluster.shards
		for i, shard := range newCluster.shards {
			if shard.currentSize/mbBytes > args.ShardSize {
				return fmt.Errorf("shards capacity cannot be reduced to %vmb, because one of the shard's current size is %vmb", args.ShardSize, shard.currentSize/mbBytes)
			}
			newCluster.shards[i].size = args.ShardSize * mbBytes
		}
	}

	newCluster.ShardSize = args.ShardSize
	newCluster.ShardsAmount = len(newCluster.shards)

	debug.SetGCPercent(GCPercentRatio(args.ShardsAmount, args.ShardSize))
	cluster = newCluster
	return nil
}

//CacheEquals ...
func CacheEquals(cacheHash *string, incomingArgs *CacheClusterArgs) bool {
	serialized, _ := json.Marshal(incomingArgs)
	md := md5.Sum(serialized)
	newCacheHash := hex.EncodeToString(md[:16])
	if *cacheHash == newCacheHash {
		return true
	}
	*cacheHash = newCacheHash
	return false
}

//TryServeFromCache ...
func TryServeFromCache(w http.ResponseWriter, r *http.Request) error {
	configuration := configutil.GetConfig()
	if ok, _ := ContainsRule(r.URL.String(), configuration.CacheRules); ok {
		cache := GetCluster()

		response, err := cache.Get(r.URL.String(), false)
		if err == nil {
			ServeFromCache(w, r, response)
			return nil
		}

		hashedKey := cache.Hash.Sum(r.URL.String())
		guard := cache.Queue.Get(hashedKey)
		//If there is no queue for a given key – create queue and set release on timeout.
		//Timeout should prevent situation when release won't be triggered in modifyResponse
		//due to server timeouts
		if guard == nil {
			cache.Queue.Set(hashedKey)
			go func() {
				for {
					select {
					case <-time.After(time.Duration(configuration.WriteTimeout) * time.Second):
						cache.Queue.Release(hashedKey)
						return
					}
				}
			}()
		} else {
			//If there is a queue for a given key – wait for it to be released and get the response
			//from the cache. Optimistically we don't need to check the returned error in this case,
			//because the only error is a "key not found" yet we immediatelly grab the value after
			//cache set.
			guard.Wait()
			response, _ := cache.Get(r.URL.String(), false)
			ServeFromCache(w, r, response)
			return nil
		}
	}

	return fmt.Errorf("%s shouldn't be cached", r.URL.Path)
}

//ContainsRule ...
func ContainsRule(path string, prefixes []*configutil.Rule) (ok bool, ttl string) {
	for _, rule := range prefixes {
		if strings.HasPrefix(path, rule.Path) {
			return true, rule.TTL
		}
	}
	return false, ""
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
