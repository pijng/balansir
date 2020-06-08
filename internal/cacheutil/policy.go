package cacheutil

import (
	"balansir/internal/helpers"
	"errors"
	"sort"
	"sync"
	"time"
)

const (
	_LRU  = "LRU"
	_MRU  = "MRU"
	_LFU  = "LFU"
	_MFU  = "MFU"
	_FiFo = "FIFO"
)

//Meta ...
type Meta struct {
	hashMap    map[uint64]HashValue
	policyType string
	mux        sync.RWMutex
}

//HashValue ...
type HashValue struct {
	value     int64
	itemIndex int
	keyIndex  uint64
	ttl       string
}

//NewMeta ...
func NewMeta(policyType string) *Meta {
	return &Meta{
		hashMap:    make(map[uint64]HashValue),
		policyType: policyType,
	}
}

func (meta *Meta) getEvictionItem() (int, uint64) {
	values := make([]HashValue, 0, len(meta.hashMap))
	for _, v := range meta.hashMap {
		values = append(values, HashValue{value: v.value, itemIndex: v.itemIndex, keyIndex: v.keyIndex, ttl: v.ttl})
	}

	switch meta.policyType {
	case _MRU, _MFU:
		sort.SliceStable(values, func(i, j int) bool {
			return values[i].value > values[j].value
		})
	case _LRU, _LFU:
		sort.SliceStable(values, func(i, j int) bool {
			return values[i].value < values[j].value
		})
	case _FiFo:
	}

	return values[0].itemIndex, values[0].keyIndex
}

func (meta *Meta) push(itemIndex int, keyIndex uint64, TTL string) {
	meta.mux.Lock()
	defer meta.mux.Unlock()

	var value int64

	switch meta.policyType {
	case _MRU, _LRU:
		duration := helpers.GetDuration(TTL)
		value = time.Now().Add(duration).Unix()
	case _MFU, _LFU:
		value = 0
	default:
		value = 0
	}

	meta.hashMap[keyIndex] = HashValue{value: value, itemIndex: itemIndex, keyIndex: keyIndex, ttl: TTL}
}

func (meta *Meta) updateMetaValue(keyIndex uint64) {
	meta.mux.Lock()
	defer meta.mux.Unlock()

	metaHash := meta.hashMap[keyIndex]

	switch meta.policyType {
	case _LFU, _MFU:
		metaHash.value++
	case _LRU, _MRU:
		duration := helpers.GetDuration(metaHash.ttl)
		metaHash.value = time.Now().Add(duration).Unix()
	}

	meta.hashMap[keyIndex] = metaHash
}

func (meta *Meta) evict() (int, uint64, error) {
	meta.mux.Lock()
	defer meta.mux.Unlock()
	if len(meta.hashMap) > 0 {

		itemIndex, keyIndex := meta.getEvictionItem()
		delete(meta.hashMap, keyIndex)

		return itemIndex, keyIndex, nil
	}
	return 0, 0, errors.New("can't evict from empty valueMap")
}

//TimeBased ...
func (meta *Meta) TimeBased() bool {
	if meta.policyType == "LRU" || meta.policyType == "MRU" {
		return true
	}
	return false
}
