package cacheutil

import (
	"encoding/binary"
	"errors"
	"sort"
	"sync"
	"time"
)

const (
	valueEntrySize = 4
	indexEntrySize = 4
	keyEntrySize   = 8
	_LRU           = "LRU"
	_LFU           = "LFU"
)

//Meta ...
type Meta struct {
	valueMap        [][]byte
	hashMap         map[uint32]*[]byte
	tmpBuffer       []byte
	valueBuffer     []byte
	itemIndexBuffer []byte
	keyIndexBuffer  []byte
	policyType      string
	mux             sync.RWMutex
}

//NewMeta ...
func NewMeta(policyType string) *Meta {
	return &Meta{
		valueMap:        make([][]byte, 0),
		hashMap:         make(map[uint32]*[]byte),
		tmpBuffer:       make([]byte, valueEntrySize+indexEntrySize+keyEntrySize),
		valueBuffer:     make([]byte, valueEntrySize),
		itemIndexBuffer: make([]byte, indexEntrySize),
		keyIndexBuffer:  make([]byte, keyEntrySize),
		policyType:      policyType,
	}
}

func (meta *Meta) sort() {
	sort.SliceStable(meta.valueMap, func(i, j int) bool {
		return binary.LittleEndian.Uint32(meta.valueMap[i][:valueEntrySize]) < binary.LittleEndian.Uint32(meta.valueMap[j][:valueEntrySize])
	})
}

func (meta *Meta) push(itemIndex uint32, keyIndex uint64) {
	meta.mux.Lock()
	defer meta.mux.Unlock()

	binary.LittleEndian.PutUint32(meta.valueBuffer, 0)
	binary.LittleEndian.PutUint32(meta.itemIndexBuffer, itemIndex)
	binary.LittleEndian.PutUint64(meta.keyIndexBuffer, keyIndex)

	copy(meta.tmpBuffer[0:], meta.valueBuffer)
	copy(meta.tmpBuffer[valueEntrySize:], meta.itemIndexBuffer)
	copy(meta.tmpBuffer[valueEntrySize+indexEntrySize:], meta.keyIndexBuffer)

	meta.valueMap = append(meta.valueMap, meta.tmpBuffer)

	meta.hashMap[itemIndex] = &meta.valueMap[len(meta.valueMap)-1]
	meta.sort()
}

func (meta *Meta) updateMetaValue(itemIndex uint32) {
	meta.mux.Lock()
	defer meta.mux.Unlock()

	valueBuffer := *meta.hashMap[itemIndex]

	switch meta.policyType {
	case _LRU:
		hitValue := binary.LittleEndian.Uint32(valueBuffer[:valueEntrySize])
		binary.LittleEndian.PutUint32(meta.valueBuffer, hitValue+1)
	case _LFU:
		timeValue := time.Now().Unix()
		binary.LittleEndian.PutUint32(meta.valueBuffer, uint32(timeValue))
	}

	valueBuffer = append(meta.valueBuffer, valueBuffer[valueEntrySize:]...)
	*meta.hashMap[itemIndex] = valueBuffer
	meta.sort()
}

func (meta *Meta) evict() (uint32, uint64, error) {
	meta.mux.Lock()
	defer meta.mux.Unlock()
	if len(meta.valueMap) > 0 {
		itemIndex := binary.LittleEndian.Uint32(meta.valueMap[0][valueEntrySize : valueEntrySize+indexEntrySize])
		keyIndex := binary.LittleEndian.Uint64(meta.valueMap[0][valueEntrySize+indexEntrySize : valueEntrySize+indexEntrySize+keyEntrySize])

		delete(meta.hashMap, itemIndex)
		meta.valueMap = meta.valueMap[1:]

		return itemIndex, keyIndex, nil
	}
	return 0, 0, errors.New("can't evict from empty hitsMap")
}
