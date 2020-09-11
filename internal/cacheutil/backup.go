package cacheutil

import (
	"balansir/internal/logutil"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	snapshotPath = ".snapshot.gob"
)

//Snapshot ...
type Snapshot struct {
	Shards    []*Shard
	KsHashMap map[uint64]string
}

//TakeCacheSnapshot ...
func TakeCacheSnapshot(cluster *CacheCluster) {
	cluster.snapshotFile.Seek(0, io.SeekStart)
	cluster.snapshotFile.Truncate(0)

	snapshot := &Snapshot{
		Shards: cluster.shards,
	}

	if cluster.backgroundUpdate {
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
			value := snapshotShard.Items[item.Index]

			err := RestoreShard(key, item, value, shard)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			shard.Tail++
			shard.CurrentSize += item.Length

			if cluster.backgroundUpdate {
				cluster.updater.keyStorage.hashmap[key] = snapshot.KsHashMap[key]
			}
		}
	}

	return errs
}

//RestoreShard ...
func RestoreShard(key uint64, item shardItem, value []byte, shard *Shard) error {
	if item.Length >= shard.size {
		return fmt.Errorf("value size is bigger than shard max size: %vmb out of %vmb", fmt.Sprintf("%.2f", float64(item.Length)/1024/1024), shard.size/1024/1024)
	}
	if shard.CurrentSize+item.Length >= shard.size {
		return errors.New("potential exceeding of shard max capacity")
	}

	shard.Hashmap[key] = item
	shard.Items[item.Index] = value

	return nil
}
