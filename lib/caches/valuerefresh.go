package caches

import (
	"sync"
	"time"
)

type ValueRefresh[T any] struct {
	lock            sync.Mutex
	lastRefresh     time.Time
	refreshInterval time.Duration
	value           T
	getValueFn      func() T
}

func NewValueRefresh[T any](interval time.Duration, getValueFn func() T) *ValueRefresh[T] {
	return &ValueRefresh[T]{
		refreshInterval: interval,
		getValueFn:      getValueFn,
	}
}

func (r *ValueRefresh[T]) Get() T {
	r.lock.Lock()
	defer r.lock.Unlock()
	if time.Since(r.lastRefresh) < r.refreshInterval {
		return r.value
	}
	r.value = r.getValueFn()
	r.lastRefresh = time.Now()
	return r.value
}

type ValueCacheCommon interface {
	Ready() bool
	Refresh()
	LastRefresh() time.Time
}

func (r *ValueRefresh[T]) LastRefresh() time.Time {
	r.lock.Lock()
	ret := r.lastRefresh
	r.lock.Unlock()
	return ret
}

func (r *ValueRefresh[T]) Ready() bool {
	r.lock.Lock()
	ret := time.Since(r.lastRefresh)+1*time.Second < r.refreshInterval
	r.lock.Unlock()
	return ret
}

func (r *ValueRefresh[T]) Refresh() {
	r.lock.Lock()
	defer r.lock.Unlock()
	if time.Since(r.lastRefresh) < r.refreshInterval {
		return
	}
	r.value = r.getValueFn()
	r.lastRefresh = time.Now()
}
