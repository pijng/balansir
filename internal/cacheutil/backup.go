package cacheutil

import (
	"balansir/internal/logutil"
	"encoding/gob"
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
				bm.takeCacheSnapshot()
			}
		case <-ticker5m.C:
			actions := bm.GetHitsCount()
			if actions > actionsThreshold15m && actions <= actionsThreshold1m {
				bm.takeCacheSnapshot()
			}
		case <-ticker15m.C:
			actions := bm.GetHitsCount()
			if actions <= actionsThreshold15m {
				bm.takeCacheSnapshot()
			}
		}
	}
}

func (bm *BackupManager) takeCacheSnapshot() {
	cache := GetCluster()
	cache.Mux.Lock()
	defer cache.Mux.Unlock()

	file, err := os.OpenFile(snapshotPath, os.O_CREATE|os.O_RDWR, 0660)
	if err != nil {
		logutil.Warning(fmt.Sprintf("failed to create/open cache snapshot file: %v", err))
		return
	}
	defer file.Close()

	err = file.Truncate(0)
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
		Shards:    cluster.shards,
		KsHashMap: cluster.updater.keyStorage.hashmap,
	}

	bm.Reset()
	encoder := gob.NewEncoder(file)
	err = encoder.Encode(snapshot)
	if err != nil {
		logutil.Warning(fmt.Sprintf("Error while saving cache on disk: %v", err))
	}
}

//GetSnapshot ...
func GetSnapshot() (*Snapshot, *os.File, error) {
	file, err := os.OpenFile(snapshotPath, os.O_CREATE|os.O_RDWR, 0660)
	if err != nil {
		return &Snapshot{}, nil, fmt.Errorf("failed to create/open cache snapshot file: %w", err)
	}

	decoder := gob.NewDecoder(file)

	snapshot := Snapshot{}
	decoder.Decode(&snapshot) //nolint

	return &snapshot, file, nil
}

//RestoreCache ...
func RestoreCache() {
	cache := GetCluster()
	cache.Mux.Lock()
	defer cache.Mux.Unlock()

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

	cluster.shards = snapshot.Shards
	cluster.updater.keyStorage.hashmap = snapshot.KsHashMap

	logutil.Notice("Cache loaded from disk")
}
