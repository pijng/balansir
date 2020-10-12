package cacheutil

import (
	"balansir/internal/logutil"
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"time"
)

const (
	snapshotPath        = ".snapshot.gob"
	actionsThreshold1m  = 100
	actionsThreshold15m = 1
)

//BackupManager ...
type BackupManager struct {
	ActionsCount int64
}

//Snapshot ...
type Snapshot struct {
	Shards    []*Shard
	KsHashMap map[uint64]string
}

//Hit ...
func (bm *BackupManager) Hit() {
	atomic.AddInt64(&bm.ActionsCount, 1)
}

//Reset ...
func (bm *BackupManager) Reset() {
	atomic.StoreInt64(&bm.ActionsCount, 0)
}

//GetHitsCount ...
func (bm *BackupManager) GetHitsCount() int64 {
	return atomic.LoadInt64(&bm.ActionsCount)
}

//PersistCache ...
func (bm *BackupManager) PersistCache() {
	ticker1m := time.NewTicker(1 * time.Minute)
	ticker5m := time.NewTicker(5 * time.Minute)
	ticker15m := time.NewTicker(15 * time.Minute)

	for {
		select {
		case <-ticker1m.C:
			actions := bm.GetHitsCount()
			if actions >= actionsThreshold1m {
				TakeCacheSnapshot()
			}
			bm.Reset()
		case <-ticker5m.C:
			actions := bm.GetHitsCount()
			if actions > actionsThreshold15m && actions <= actionsThreshold1m {
				TakeCacheSnapshot()
			}
			bm.Reset()
		case <-ticker15m.C:
			actions := bm.GetHitsCount()
			if actions <= actionsThreshold15m {
				TakeCacheSnapshot()
			}
			bm.Reset()
		}
	}
}

//TakeCacheSnapshot ...
func TakeCacheSnapshot() {
	file, err := os.OpenFile(snapshotPath, os.O_CREATE|os.O_RDWR, 0660)
	if err != nil {
		logutil.Warning(fmt.Errorf("failed to create/open cache snapshot file: %v", err))
		return
	}
	defer file.Close()

	file.Truncate(0)
	if err != nil {
		logutil.Warning(fmt.Sprintf("Error while saving cache on disk: %v", err))
		return
	}

	_, err = file.Seek(0, 0)
	if err != nil {
		logutil.Warning(fmt.Sprintf("Error while saving cache on disk: %v", err))
		return
	}

	snapshot := &Snapshot{
		Shards: cluster.shards,
	}

	if cluster.backgroundUpdate && cluster.updater != nil {
		snapshot.KsHashMap = cluster.updater.keyStorage.hashmap
	}

	encoder := gob.NewEncoder(file)
	err = encoder.Encode(&snapshot)
	if err != nil {
		logutil.Warning(fmt.Sprintf("Error while saving cache on disk: %v", err))
	}
}

//GetSnapshot ...
func GetSnapshot() (Snapshot, *os.File, error) {
	file, err := os.OpenFile(snapshotPath, os.O_CREATE|os.O_RDWR, 0660)
	if err != nil {
		return Snapshot{}, nil, fmt.Errorf("failed to create/open cache snapshot file: %v", err)
	}

	decoder := gob.NewDecoder(file)

	snapshot := Snapshot{}
	decoder.Decode(&snapshot) //nolint

	return snapshot, file, nil
}

//RestoreCache ...
func RestoreCache(cluster *CacheCluster) {
	snapshot, file, err := GetSnapshot()
	defer file.Close()

	if err != nil {
		logutil.Warning(err)
		return
	}

	stats, err := file.Stat()
	if err != nil {
		logutil.Warning(err)
	}

	if stats.Size() == 0 {
		return
	}

	errs := RestoreShards(cluster, snapshot, cluster.shards)
	if errs != nil {
		logutil.Warning("Encountered the following errors while loading cache from disk:")
		for _, err := range errs {
			logutil.Warning(err)
		}
		return
	}

	logutil.Notice("Cache loaded from disk")
}

//RestoreShards ...
func RestoreShards(cluster *CacheCluster, snapshot Snapshot, shards []*Shard) []error {
	var errs []error

	for _, snapshotShard := range snapshot.Shards {
		for key, item := range snapshotShard.Hashmap {
			shard := cluster.getShard(key)

			err := RestoreShard(key, item, shard, snapshotShard)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			if cluster.backgroundUpdate {
				cluster.updater.keyStorage.SetHashedKey(snapshot.KsHashMap[key], key)
			}
		}
	}

	return errs
}

//RestoreShard ...
func RestoreShard(hashedKey uint64, item shardItem, shard *Shard, snapshotShard *Shard) error {
	if item.Length >= shard.size {
		return fmt.Errorf("value size is bigger than shard max size: %vmb out of %vmb", fmt.Sprintf("%.2f", float64(item.Length)/1024/1024), shard.size/1024/1024)
	}
	if shard.CurrentSize+item.Length >= shard.size {
		return errors.New("potential exceeding of shard max capacity")
	}

	value := snapshotShard.Items[item.Index]
	index := shard.push(value)
	shard.Hashmap[hashedKey] = shardItem{Index: index, Length: len(value), TTL: item.TTL}

	if shard.Policy != nil && snapshotShard.Policy != nil {
		TTL := snapshotShard.Policy.HashMap[hashedKey].TTL
		shard.Policy.push(index, hashedKey, TTL)
	}

	return nil
}
