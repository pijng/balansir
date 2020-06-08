package cacheutil

import (
	"sync"
)

//Queue ...
type Queue struct {
	hashMap map[uint64]*sync.WaitGroup
	mux     sync.RWMutex
}

//NewQueue ...
func NewQueue() *Queue {
	return &Queue{
		hashMap: make(map[uint64]*sync.WaitGroup),
	}
}

//Set ...
func (q *Queue) Set(hashedKey uint64) bool {
	q.mux.Lock()
	defer q.mux.Unlock()

	_, ok := q.hashMap[hashedKey]
	if ok {
		return false
	}

	q.hashMap[hashedKey] = &sync.WaitGroup{}
	q.hashMap[hashedKey].Add(1)
	return true
}

//Release ...
func (q *Queue) Release(hashedKey uint64) {
	q.mux.Lock()
	defer q.mux.Unlock()

	q.hashMap[hashedKey].Done()
	delete(q.hashMap, hashedKey)
}

//Get ...
func (q *Queue) Get(hashedKey uint64) *sync.WaitGroup {
	q.mux.Lock()
	defer q.mux.Unlock()

	guard := q.hashMap[hashedKey]

	return guard
}
