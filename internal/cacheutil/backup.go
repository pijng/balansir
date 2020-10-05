package cacheutil

import (
	"balansir/internal/logutil"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
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

//PersistCache ...
func (bm *BackupManager) PersistCache() {
	ticker1m := time.NewTicker(1 * time.Minute)
	ticker5m := time.NewTicker(5 * time.Minute)
	ticker15m := time.NewTicker(15 * time.Minute)

	for {
		select {
		case <-ticker1m.C:
			actions := atomic.LoadInt64(&bm.ActionsCount)
			if actions >= actionsThreshold1m {
				TakeCacheSnapshot()
			}
			atomic.StoreInt64(&bm.ActionsCount, 0)
		case <-ticker5m.C:
			actions := atomic.LoadInt64(&bm.ActionsCount)
			if actions > actionsThreshold15m && actions <= actionsThreshold1m {
				TakeCacheSnapshot()
			}
			atomic.StoreInt64(&bm.ActionsCount, 0)
		case <-ticker15m.C:
			actions := atomic.LoadInt64(&bm.ActionsCount)
			if actions <= actionsThreshold15m {
				TakeCacheSnapshot()
			}
			atomic.StoreInt64(&bm.ActionsCount, 0)
		}
	}
}

//Hit ...
func (bm *BackupManager) Hit() {
	atomic.AddInt64(&bm.ActionsCount, 1)
}

//TakeCacheSnapshot ...
func TakeCacheSnapshot() {
	cluster := GetCluster()

	cluster.snapshotFile.Truncate(0)
	cluster.snapshotFile.Seek(0, io.SeekStart)

	snapshot := &Snapshot{
		Shards: cluster.shards,
	}

	if cluster.backgroundUpdate && cluster.updater != nil {
		snapshot.KsHashMap = cluster.updater.keyStorage.hashmap
	}

	err := cluster.encoder.Encode(&snapshot)
	if err != nil {
		logutil.Warning(fmt.Sprintf("Error while processing cache backup: %v", err))
	}
}

//GetSnapshot ...
func GetSnapshot() (Snapshot, *gob.Encoder, *os.File, error) {
	bf, err := os.OpenFile(snapshotPath, os.O_CREATE|os.O_RDWR, 0660)
	if err != nil {
		return Snapshot{}, nil, nil, fmt.Errorf("failed to create/open cache snapshot file: %v", err)
	}

	encoder := gob.NewEncoder(bf)
	decoder := gob.NewDecoder(bf)

	snapshot := Snapshot{}
	err = decoder.Decode(&snapshot)

	return snapshot, encoder, bf, nil
}

//RestoreCache ...
func RestoreCache(cluster *CacheCluster) {
	snapshot, encoder, file, err := GetSnapshot()
	if err != nil {
		logutil.Warning(err)
		return
	}

	cluster.encoder = encoder
	cluster.snapshotFile = file

	stats, err := file.Stat()
	if err != nil {
		logutil.Warning(err)
	}

	if stats.Size() == 0 {
		return
	}

	logutil.Info("Processing cache backup...")

	errs := RestoreShards(cluster, snapshot, cluster.shards)
	if errs != nil {
		logutil.Warning("Encountered the following errors while processing cache backup")
		for i := 0; i < len(errs); i++ {
			logutil.Warning(fmt.Sprintf("\t %v", errs[i]))
		}
		return
	}

	logutil.Notice("Cache backup succeeded")
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
