package cacheutil

import (
	"encoding/binary"
	"errors"
	"sort"
	"sync"
)

const (
	hitsEntrySize  = 4
	indexEntrySize = 4
	keyEntrySize   = 8
)

//LFU ...
type LFU struct {
	hitsmap         [][]byte
	hashmap         map[uint32]*[]byte
	hitsBuffer      []byte
	itemIndexBuffer []byte
	keyIndexBuffer  []byte
	mux             sync.RWMutex
}

//NewLFUMeta ...
func NewLFUMeta() *LFU {
	return &LFU{
		hitsmap:         make([][]byte, 0),
		hashmap:         make(map[uint32]*[]byte),
		hitsBuffer:      make([]byte, hitsEntrySize),
		itemIndexBuffer: make([]byte, indexEntrySize),
		keyIndexBuffer:  make([]byte, keyEntrySize),
	}
}

func (meta *LFU) sort() {
	sort.SliceStable(meta.hitsmap, func(i, j int) bool {
		return binary.LittleEndian.Uint32(meta.hitsmap[i][:hitsEntrySize]) < binary.LittleEndian.Uint32(meta.hitsmap[j][:hitsEntrySize])
	})
}

func (meta *LFU) push(itemIndex uint32, keyIndex uint64) {
	meta.mux.Lock()
	defer meta.mux.Unlock()

	binary.LittleEndian.PutUint32(meta.hitsBuffer, 0)
	binary.LittleEndian.PutUint32(meta.itemIndexBuffer, itemIndex)
	binary.LittleEndian.PutUint64(meta.keyIndexBuffer, keyIndex)
	tmpBuffer := make([]byte, 0)
	tmpBuffer = append(meta.hitsBuffer, meta.itemIndexBuffer...)
	tmpBuffer = append(tmpBuffer, meta.keyIndexBuffer...)
	meta.hitsmap = append(meta.hitsmap, tmpBuffer)
	meta.hashmap[itemIndex] = &meta.hitsmap[len(meta.hitsmap)-1]
	meta.sort()
}

func (meta *LFU) hit(itemIndex uint32) {
	meta.mux.Lock()
	defer meta.mux.Unlock()

	hitsBuffer := *meta.hashmap[itemIndex]

	hits := binary.LittleEndian.Uint32(hitsBuffer[:hitsEntrySize])
	binary.LittleEndian.PutUint32(meta.hitsBuffer, hits+1)
	hitsBuffer = append(meta.hitsBuffer, hitsBuffer[hitsEntrySize:]...)
	*meta.hashmap[itemIndex] = hitsBuffer
	meta.sort()
}

func (meta *LFU) evict() (uint32, uint64, error) {
	meta.mux.Lock()
	defer meta.mux.Unlock()
	if len(meta.hitsmap) > 0 {
		itemIndex := binary.LittleEndian.Uint32(meta.hitsmap[0][hitsEntrySize : hitsEntrySize+indexEntrySize])
		keyIndex := binary.LittleEndian.Uint64(meta.hitsmap[0][hitsEntrySize+indexEntrySize : hitsEntrySize+indexEntrySize+keyEntrySize])
		delete(meta.hashmap, itemIndex)
		meta.hitsmap = meta.hitsmap[1:]
		return itemIndex, keyIndex, nil
	}
	return 0, 0, errors.New("can't evict from empty hitsmap")
}
