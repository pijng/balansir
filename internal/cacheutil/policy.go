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
	_MRU           = "MRU"
	_LFU           = "LFU"
	_MFU           = "MFU"
	_FiFo          = "FIFO"
)

//Meta ...
type Meta struct {
	valueMap    [][]byte
	hashMap     map[uint32]*[]byte
	valueBuffer []byte
	policyType  string
	mux         sync.RWMutex
}

//NewMeta ...
func NewMeta(policyType string) *Meta {
	return &Meta{
		valueMap:    make([][]byte, 0),
		hashMap:     make(map[uint32]*[]byte),
		valueBuffer: make([]byte, valueEntrySize),
		policyType:  policyType,
	}
}

func (meta *Meta) sort() {
	switch meta.policyType {
	case _MRU, _MFU:
		sort.SliceStable(meta.valueMap, func(i, j int) bool {
			return binary.LittleEndian.Uint32(meta.valueMap[i][:valueEntrySize]) > binary.LittleEndian.Uint32(meta.valueMap[j][:valueEntrySize])
		})
	case _LRU, _LFU:
		sort.SliceStable(meta.valueMap, func(i, j int) bool {
			return binary.LittleEndian.Uint32(meta.valueMap[i][:valueEntrySize]) < binary.LittleEndian.Uint32(meta.valueMap[j][:valueEntrySize])
		})
	case _FiFo:
		return
	}
}

func (meta *Meta) push(itemIndex uint32, keyIndex uint64) {
	meta.mux.Lock()
	defer meta.mux.Unlock()

	tmpBuffer := make([]byte, valueEntrySize+indexEntrySize+keyEntrySize)
	valueBuffer := make([]byte, valueEntrySize)
	itemIndexBuffer := make([]byte, indexEntrySize)
	keyIndexBuffer := make([]byte, keyEntrySize)
	binary.LittleEndian.PutUint32(valueBuffer, 0)
	binary.LittleEndian.PutUint32(itemIndexBuffer, itemIndex)
	binary.LittleEndian.PutUint64(keyIndexBuffer, keyIndex)

	copy(tmpBuffer[0:], valueBuffer)
	copy(tmpBuffer[valueEntrySize:], itemIndexBuffer)
	copy(tmpBuffer[valueEntrySize+indexEntrySize:], keyIndexBuffer)

	meta.valueMap = append(meta.valueMap, tmpBuffer)

	meta.hashMap[itemIndex] = &meta.valueMap[len(meta.valueMap)-1]
	meta.sort()
}

func (meta *Meta) updateMetaValue(itemIndex uint32) {
	meta.mux.Lock()
	defer meta.mux.Unlock()

	valueBuffer := *meta.hashMap[itemIndex]
	var newValue uint32

	switch meta.policyType {
	case _LFU, _MFU:
		newValue = binary.LittleEndian.Uint32(valueBuffer[:valueEntrySize]) + 1
	case _LRU, _MRU:
		newValue = uint32(time.Now().Unix())
	}
	binary.LittleEndian.PutUint32(meta.valueBuffer, newValue)

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
