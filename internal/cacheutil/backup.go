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
	cachePath = ".cache.gob"
)

//Backup ...
type Backup struct {
	Shards    []*Shard
	KsHashMap map[uint64]string
}

//BackupCache ...
func BackupCache(cluster *CacheCluster) {
	cluster.backup.Seek(0, io.SeekStart)
	cluster.backup.Truncate(0)

	backup := &Backup{
		Shards: cluster.shards,
	}

	if cluster.backgroundUpdate {
		backup.KsHashMap = cluster.updater.keyStorage.hashmap
	}

	err := cluster.encoder.Encode(&backup)
	if err != nil {
		logutil.Warning(fmt.Sprintf("Error while processing cache backup: %v", err))
	}
}

//GetBackup ...
func GetBackup() (Backup, *gob.Encoder, *os.File, error) {
	bf, err := os.OpenFile(cachePath, os.O_CREATE|os.O_RDWR, 0660)
	if err != nil {
		return Backup{}, nil, nil, fmt.Errorf("failed to create/open cache backup file: %v", err)
	}

	encoder := gob.NewEncoder(bf)
	decoder := gob.NewDecoder(bf)

	backup := Backup{}
	err = decoder.Decode(&backup)

	return backup, encoder, bf, nil
}

//RestoreCache ...
func RestoreCache(cluster *CacheCluster) {
	backup, encoder, file, err := GetBackup()
	if err != nil {
		logutil.Warning(err)
		return
	}

	cluster.encoder = encoder
	cluster.backup = file

	stats, err := file.Stat()
	if err != nil {
		logutil.Warning(err)
	}

	if stats.Size() == 0 {
		return
	}

	logutil.Info("Restoring cache backup...")

	errs := RestoreShards(cluster, backup, cluster.shards)
	if errs != nil {
		logutil.Warning("Encountered the following errors while processing cache restore")
		for i := 0; i < len(errs); i++ {
			logutil.Warning(fmt.Sprintf("\t %v", errs[i]))
		}
		return
	}

	logutil.Notice("Cache backup restored")
}

//RestoreShards ...
func RestoreShards(cluster *CacheCluster, backup Backup, shards []*Shard) []error {
	var errs []error

	for _, backupShard := range backup.Shards {
		for key, item := range backupShard.Hashmap {
			shard := cluster.getShard(key)
			value := backupShard.Items[item.Index]

			err := RestoreShard(key, item, value, shard)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			if cluster.backgroundUpdate {
				cluster.updater.keyStorage.hashmap[key] = backup.KsHashMap[key]
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
	if shard.currentSize+item.Length >= shard.size {
		return errors.New("potential exceeding of shard max capacity")
	}

	shard.Hashmap[key] = item
	shard.Items[item.Index] = value

	return nil
}
