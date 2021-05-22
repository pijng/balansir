package cacheutil

import (
	"balansir/internal/configutil"
	"balansir/internal/logutil"
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	offset64 = 14695981039346656037
	prime64  = 1099511628211
	mbBytes  = 1048576

	pow2 = float64(int64(1) << 31)
)

var (
	//KeyValueDelimeter ...
	KeyValueDelimeter = []byte(";balansir-key-value-delimeter;")
	//PairDelimeter ...
	PairDelimeter = []byte(";balansir-pair-delimeter;")
	//HeadersDelimeter ...
	HeadersDelimeter = []byte(";balansir-headers-delimeter;")
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
	backupManager    *BackupManager
	shards           []*Shard
	Hash             fnv64a
	ShardsAmount     int
	ShardSize        int
	Queue            *Queue
	Hits             int64
	Misses           int64
	backgroundUpdate bool
	updater          *Updater
	cacheRules       []*configutil.Rule
	Mux              sync.RWMutex
}

//CacheClusterArgs ...
type CacheClusterArgs struct {
	ShardsAmount     int
	ShardSize        int
	CachePolicy      string
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
		backupManager:    &BackupManager{},
		shards:           make([]*Shard, args.ShardsAmount),
		ShardsAmount:     args.ShardsAmount,
		ShardSize:        args.ShardSize,
		Queue:            NewQueue(),
		cacheRules:       args.CacheRules,
		backgroundUpdate: args.BackgroundUpdate,
		updater:          NewUpdater(args.Port, args.TransportTimeout, args.DialerTimeout),
	}

	for i := 0; i < args.ShardsAmount; i++ {
		cluster.shards[i] = CreateShard(args.ShardSize*mbBytes, args.CachePolicy)
	}

	go RestoreCache()
	go cluster.runInvalidation()
	go cluster.backupManager.PersistCache()

	return cluster
}

//GetCluster ...
func GetCluster() *CacheCluster {
	return cluster
}

//https://arxiv.org/pdf/1406.2294.pdf
//grsky golang implementation https://github.com/dgryski/go-jump/blob/master/jump.go
func jumpConsistentHash(hashedKey uint64, shardsAmount int) int64 {
	var b int64 = -1
	var j int64

	for j < int64(shardsAmount) {
		b = j
		hashedKey = hashedKey*2862933555777941757 + 1
		j = int64(float64(b+1) * (pow2 / float64((hashedKey>>33)+1)))
	}

	return b
}

func (cluster *CacheCluster) getShard(hashedKey uint64) *Shard {
	index := jumpConsistentHash(hashedKey, cluster.ShardsAmount)
	return cluster.shards[index]
}

//Set ...
func (cluster *CacheCluster) Set(key string, value []byte, TTL string) (err error) {
	hashedKey := cluster.Hash.Sum(key)
	shard := cluster.getShard(hashedKey)
	shard.mux.Lock()
	defer shard.mux.Unlock()

	if len(value) > shard.Size {
		return fmt.Errorf("value size is bigger than shard max size: %vmb out of %vmb", fmt.Sprintf("%.2f", float64(len(value)/mbBytes)), shard.Size/mbBytes)
	}

	if shard.CurrentSize+len(value) >= shard.Size {
		if err := shard.evict(len(value)); err != nil {
			return err
		}
	}

	shard.set(hashedKey, value, TTL)
	cluster.updater.keyStorage.SetHashedKey(key, hashedKey)

	cluster.backupManager.Hit()

	return nil
}

//Get ...
func (cluster *CacheCluster) Get(key string, trackMisses bool) ([]byte, error) {
	hashedKey := cluster.Hash.Sum(key)
	shard := cluster.getShard(hashedKey)
	value, err := shard.get(hashedKey)

	if err == nil {
		cluster.hit()
		shard.Policy.updateMetaValue(hashedKey)
	}

	if err != nil && trackMisses {
		cluster.miss()
	}

	return value, err
}

func (cluster *CacheCluster) invalidate(timestamp int64) {
	for _, shard := range cluster.shards {
		shard.update(timestamp, cluster.updater)
	}
}

func (cluster *CacheCluster) runInvalidation() {
	timer := time.NewTicker(1 * time.Second)
	for {
		<-timer.C
		cluster.invalidate(time.Now().Unix())
	}
}

//ServeFromCache ...
func ServeFromCache(w http.ResponseWriter, r *http.Request, value []byte) {
	slicedValue := bytes.Split(value, HeadersDelimeter)
	headers := slicedValue[0]
	body := slicedValue[1]

	headersPairs := bytes.Split(headers, PairDelimeter)
	for _, pair := range headersPairs {
		slicedPair := bytes.Split(pair, KeyValueDelimeter)
		for i := range slicedPair {
			//Prevent writing last pair value as a separate key
			if i+1 <= len(slicedPair)-1 {
				w.Header().Set(string(slicedPair[i]), string(slicedPair[i+1]))
			}
		}
	}

	bodyBuf := bytes.NewBuffer([]byte{})
	bodyBuf.Write(body)

	_, err := w.Write(bodyBuf.Bytes())

	if err != nil {
		logutil.Error(err)
	}
}

//GetHitRatio ...
func (cluster *CacheCluster) GetHitRatio() float64 {
	hits := float64(cluster.getHits())
	misses := float64(cluster.getMisses())
	return (hits / math.Max(hits+misses, 1)) * 100
}

func (cluster *CacheCluster) hit() {
	atomic.AddInt64(&cluster.Hits, 1)
}

func (cluster *CacheCluster) getHits() int64 {
	return atomic.LoadInt64(&cluster.Hits)
}

func (cluster *CacheCluster) miss() {
	atomic.AddInt64(&cluster.Misses, 1)
}

func (cluster *CacheCluster) getMisses() int64 {
	return atomic.LoadInt64(&cluster.Misses)
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
		backupManager:    cluster.backupManager,
		Hits:             cluster.Hits,
		Misses:           cluster.Misses,
		ShardsAmount:     cluster.ShardsAmount,
		ShardSize:        args.ShardSize,
		shards:           cluster.shards,
		Queue:            cluster.Queue,
		backgroundUpdate: args.BackgroundUpdate,
		cacheRules:       args.CacheRules,
		updater:          cluster.updater,
	}

	if cluster.ShardsAmount != args.ShardsAmount {
		newCluster.shards = DistributeShards(newCluster, cluster, args)
	}

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
	mustBeCached, _ := ContainsRule(r.URL.String(), configuration.Cache.Rules)

	if !mustBeCached {
		return fmt.Errorf("%s shouldn't be cached", r.URL.Path)
	}

	cache := GetCluster()
	response, err := cache.Get(r.URL.String(), false)
	if err == nil {
		ServeFromCache(w, r, response)
		return nil
	}

	hashedKey := cache.Hash.Sum(r.URL.String())
	transaction := cache.Queue.Get(hashedKey)
	//If there is no queue for a given key – create queue and set release on timeout.
	//Timeout should prevent situation when release won't be triggered in modifyResponse
	//due to server timeouts
	if transaction == nil {
		cache.Queue.Set(hashedKey)
		go func() {
			for {
				<-time.After(time.Duration(configuration.WriteTimeout) * time.Second)
				cache.Queue.Release(hashedKey)
				return
			}
		}()
	} else {
		//If there is a queue for a given key – wait for it to be released and get the response
		//from the cache. Optimistically we don't need to check the returned error in this case,
		//because the only error is a "key not found" yet we immediatelly grab the value after
		//cache set.
		transaction.Wait()
		response, _ := cache.Get(r.URL.String(), false)
		ServeFromCache(w, r, response)
		return nil
	}

	return err
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
