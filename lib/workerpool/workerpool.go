package workerpool

import "sync"

type WorkerPool struct {
	lock       sync.Mutex
	numRunners int
	wg         sync.WaitGroup
	isClosed   bool
	tasks      chan func()
}

func NewWorkerPool(numRunners int) *WorkerPool {
	ret := &WorkerPool{
		numRunners: numRunners,
		tasks:      make(chan func(), 16),
	}
	for range numRunners {
		ret.wg.Go(ret.runner)
	}
	return ret
}

func (wp *WorkerPool) SubmitBackground(task func()) bool {
	wp.lock.Lock()
	defer wp.lock.Unlock()
	if wp.isClosed {
		return false
	}
	select {
	case wp.tasks <- task:
		return true
	default:
		return false
	}
}

func (wp *WorkerPool) SubmitAndWait(task func()) bool {
	wp.lock.Lock()
	defer wp.lock.Unlock()
	if wp.isClosed {
		return false
	}
	var done sync.WaitGroup
	done.Add(1)
	select {
	case wp.tasks <- func() {
		task()
		done.Done()
	}:
		done.Wait()
		return true
	default:
		return false
	}
}

func (wp *WorkerPool) runner() {
	for fn := range wp.tasks {
		fn()
	}
}

func (wp *WorkerPool) Close() {
	wp.lock.Lock()
	defer wp.lock.Unlock()
	wp.isClosed = true
	close(wp.tasks)
}
