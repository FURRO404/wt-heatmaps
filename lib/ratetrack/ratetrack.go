package ratetrack

import (
	"sync/atomic"
	"time"
)

type e struct {
	t time.Time
	v int
}

type RateTracker struct {
	buffer     []e
	head, tail int
	window     time.Duration
	count, sum *atomic.Int64
	runningSum int
}

func NewRateTracker(window time.Duration, count, sum *atomic.Int64) *RateTracker {
	return &RateTracker{
		buffer: make([]e, 64),
		count:  count,
		sum:    sum,
		window: window,
	}
}

func (r *RateTracker) Measure(val int) {
	now := time.Now()
	cutoff := now.Add(-r.window)
	if r.tail >= len(r.buffer) {
		newBuffer := make([]e, len(r.buffer)*2)
		copy(newBuffer, r.buffer)
		r.buffer = newBuffer
	}
	r.buffer[r.tail] = e{t: now, v: val}
	r.runningSum += val
	r.tail++
	for r.head < r.tail && r.buffer[r.head].t.Before(cutoff) {
		r.runningSum -= r.buffer[r.head].v
		r.head++
	}
	count := r.tail - r.head
	if r.head > len(r.buffer)/2 && count < len(r.buffer)/4 {
		copy(r.buffer, r.buffer[r.head:r.tail])
		r.tail = count
		r.head = 0
	}
	r.count.Store(int64(count))
	r.sum.Store(int64(r.runningSum))
}
