package cacheutil

import (
	"balansir/internal/logutil"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
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
	HashMap    map[uint64]HashValue
	PolicyType string
	mux        sync.RWMutex
}

//HashValue ...
type HashValue struct {
	Value     int64
	ItemIndex int
	KeyIndex  uint64
	TTL       string
}

//NewMeta ...
func NewMeta(policyType string) *Meta {
	return &Meta{
		HashMap:    make(map[uint64]HashValue),
		PolicyType: policyType,
	}
}

func (meta *Meta) getEvictionItem() (int, uint64) {
	values := make([]HashValue, 0, len(meta.HashMap))
	for _, v := range meta.HashMap {
		values = append(values, HashValue{Value: v.Value, ItemIndex: v.ItemIndex, KeyIndex: v.KeyIndex, TTL: v.TTL})
	}

	switch meta.PolicyType {
	case _MRU, _MFU:
		sort.SliceStable(values, func(i, j int) bool {
			return values[i].Value > values[j].Value
		})
	case _LRU, _LFU:
		sort.SliceStable(values, func(i, j int) bool {
			return values[i].Value < values[j].Value
		})
	case _FiFo:
	default:
	}

	return values[0].ItemIndex, values[0].KeyIndex
}

func (meta *Meta) push(itemIndex int, keyIndex uint64, TTL string) {
	meta.mux.Lock()
	defer meta.mux.Unlock()

	var value int64

	switch meta.PolicyType {
	case _MRU, _LRU:
		duration := getDuration(TTL)
		value = time.Now().Add(duration).Unix()
	case _MFU, _LFU:
		value = 0
	case _FiFo:
	default:
		value = 0
	}

	meta.HashMap[keyIndex] = HashValue{Value: value, ItemIndex: itemIndex, KeyIndex: keyIndex, TTL: TTL}
}

func (meta *Meta) updateMetaValue(keyIndex uint64) {
	meta.mux.Lock()
	defer meta.mux.Unlock()

	metaHash := meta.HashMap[keyIndex]

	switch meta.PolicyType {
	case _LFU, _MFU:
		metaHash.Value++
	case _LRU, _MRU:
		duration := getDuration(metaHash.TTL)
		metaHash.Value = time.Now().Add(duration).Unix()
	}

	meta.HashMap[keyIndex] = metaHash
}

func (meta *Meta) evict() (int, uint64, error) {
	meta.mux.Lock()
	defer meta.mux.Unlock()
	if len(meta.HashMap) > 0 {

		itemIndex, keyIndex := meta.getEvictionItem()
		delete(meta.HashMap, keyIndex)

		return itemIndex, keyIndex, nil
	}
	return 0, 0, errors.New("can't evict from empty valueMap")
}

//TimeBased ...
func (meta *Meta) TimeBased() bool {
	if meta.PolicyType == _LRU || meta.PolicyType == _MRU {
		return true
	}
	return false
}

func getDuration(TTL string) time.Duration {
	if TTL == "" {
		// If TTL isn't specified then return go's max time as Unix int64 value,
		// so in this case the stored response won't be evicted from cache at all.
		// See https://stackoverflow.com/a/25065327
		return 9223372036854775807
	}

	splittedTTL := strings.Split(TTL, ".")
	val, err := strconv.Atoi(splittedTTL[0])

	if err != nil {
		logutil.Warning(fmt.Sprintf("error reading cache item TTL: %s. Cache item TTL is set to infinity", err.Error()))
		return 9223372036854775807
	}
	unit := splittedTTL[1]

	var duration time.Duration
	switch strings.ToLower(unit) {
	case "second":
		duration = time.Duration(time.Duration(val) * time.Second)
	case "minute":
		duration = time.Duration(time.Duration(val) * time.Minute)
	case "hour":
		duration = time.Duration(time.Duration(val) * time.Hour)
	default:
		logutil.Warning(fmt.Sprintf("cache item TTL unit must be one of the following: Hour, Minute, Second. Got %s instead. Cache item TTL was set to infinity", strings.ToLower(unit)))
		return 9223372036854775807
	}

	return duration
}
