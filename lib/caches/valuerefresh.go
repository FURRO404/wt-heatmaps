package caches

import (
	"main/lib/workerpool"
	"sync"
	"time"
)

type ValueRefresh[T any] struct {
	lock            sync.Mutex
	lastRefresh     time.Time
	refreshInterval time.Duration
	value           T
	valueErr        error
	getValueFn      func() (T, error)
	wp              *workerpool.WorkerPool
	refreshing      bool
	refreshDone     chan struct{}
}

func NewValueRefresh[T any](wp *workerpool.WorkerPool, interval time.Duration, getValueFn func() (T, error)) *ValueRefresh[T] {
	return &ValueRefresh[T]{
		wp:              wp,
		refreshInterval: interval,
		getValueFn:      getValueFn,
	}
}

// blocks until gets refreshed value
func (r *ValueRefresh[T]) Get() (T, error) {
	r.lock.Lock()
	for r.refreshing {
		done := r.refreshDone
		r.lock.Unlock()
		<-done
		r.lock.Lock()
	}
	if time.Since(r.lastRefresh) < r.refreshInterval {
		retval, reterr := r.value, r.valueErr
		r.lock.Unlock()
		return retval, reterr
	}

	r.acquireValue()

	retval, reterr := r.value, r.valueErr
	r.lock.Unlock()
	return retval, reterr
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
	ret := !r.lastRefresh.IsZero() && time.Since(r.lastRefresh)+1*time.Second < r.refreshInterval
	r.lock.Unlock()
	return ret
}

// assumes lock is initially locked, will unlock while refreshing and keep locked once done
func (r *ValueRefresh[T]) acquireValue() {
	r.refreshing = true
	r.refreshDone = make(chan struct{})
	r.lock.Unlock()

	v, verr := r.getValueFn()

	r.lock.Lock()
	r.value = v
	r.valueErr = verr
	r.lastRefresh = time.Now()
	r.refreshing = false
	if r.refreshDone != nil {
		close(r.refreshDone)
	}
}

// blocks only if worker pool is disfunctional
func (r *ValueRefresh[T]) Refresh() {
	r.lock.Lock()
	if time.Since(r.lastRefresh) < r.refreshInterval || r.refreshing {
		r.lock.Unlock()
		return
	}
	if !r.wp.SubmitBackground(func() {
		r.acquireValue()
		r.lock.Unlock()
	}) {
		r.acquireValue()
		r.lock.Unlock()
	}
}
