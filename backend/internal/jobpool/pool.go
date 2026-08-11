// Package jobpool provides a single, app-wide worker pool shared by all media
// jobs. The pool capacity is reconfigurable at runtime: the worker pool size
// setting is the hard cap on how many work items execute concurrently, across
// every job. Jobs submit work as closures and wait for completion; a paused
// job keeps its queued work parked (without holding any pool capacity) until
// it is resumed.
package jobpool

import (
	"context"
	"sync"
)

// Work is a single unit of work a pool worker executes.
type Work func(ctx context.Context) error

// Pool is the shared worker pool. It is safe for concurrent use.
type Pool struct {
	mu      sync.Mutex
	cond    *sync.Cond
	queues  map[string]*queue
	running int
	maxSize int
	closed  bool
}

type queue struct {
	ctx     context.Context
	items   []Work
	next    int
	running int
	failed  bool
	err     error
	paused  bool
	done    bool
}

// New creates a pool with a fixed number of worker goroutines and the given
// initial capacity. Capacity can be changed later with SetCapacity; workers
// beyond the current capacity simply wait for work.
func New(maxWorkers, capacity int) *Pool {
	if maxWorkers < 1 {
		maxWorkers = 1
	}
	if capacity < 1 {
		capacity = 1
	}
	p := &Pool{
		queues:  map[string]*queue{},
		maxSize: capacity,
	}
	p.cond = sync.NewCond(&p.mu)
	for i := 0; i < maxWorkers; i++ {
		go p.worker()
	}
	return p
}

// SetCapacity resizes the pool. In-flight work finishes; new work is admitted
// up to the new capacity. Raising the capacity immediately admits more
// concurrent work, so a pool-size change takes effect on running jobs.
func (p *Pool) SetCapacity(n int) {
	if p == nil {
		return
	}
	if n < 1 {
		n = 1
	}
	p.mu.Lock()
	p.maxSize = n
	p.cond.Broadcast()
	p.mu.Unlock()
}

// Submit queues work for a job. The context is used to run the job's work
// items. paused must reflect the job's current pause state so that work
// submitted while the job is already paused stays parked.
func (p *Pool) Submit(jobID string, ctx context.Context, paused bool, work []Work) {
	if p == nil || len(work) == 0 {
		return
	}
	p.mu.Lock()
	q := p.queues[jobID]
	if q == nil {
		q = &queue{}
		p.queues[jobID] = q
	}
	q.ctx = ctx
	q.paused = paused
	q.items = append(q.items, work...)
	p.cond.Broadcast()
	p.mu.Unlock()
}

// SetJobPaused marks a job paused or running again. While paused, the job's
// queued work is not handed out to workers; it resumes once SetJobPaused(false)
// is called. The call is a no-op if the job has not submitted any work.
func (p *Pool) SetJobPaused(jobID string, paused bool) {
	if p == nil {
		return
	}
	p.mu.Lock()
	if q := p.queues[jobID]; q != nil {
		q.paused = paused
	}
	p.cond.Broadcast()
	p.mu.Unlock()
}

// CancelJob drops a job's queued work. In-flight work keeps running; Wait
// returns context.Canceled once no work remains.
func (p *Pool) CancelJob(jobID string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	if q := p.queues[jobID]; q != nil {
		q.done = true
	}
	p.cond.Broadcast()
	p.mu.Unlock()
}

// Wait blocks until all of the job's work has finished, or the first work item
// fails, or the job is cancelled (via ctx or CancelJob). Wait keeps blocking
// while the job is paused.
func (p *Pool) Wait(ctx context.Context, jobID string) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	q := p.queues[jobID]
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			p.cond.Broadcast()
		case <-stop:
		}
	}()
	defer close(stop)
	for q != nil && !q.done && (q.next < len(q.items) || q.running > 0) {
		if err := ctx.Err(); err != nil {
			q.done = true
			p.cond.Broadcast()
			return err
		}
		p.cond.Wait()
	}
	if q == nil {
		return nil
	}
	delete(p.queues, jobID)
	if q.failed {
		return q.err
	}
	return nil
}

// Close stops the pool's workers. Pending work is abandoned.
func (p *Pool) Close() {
	p.mu.Lock()
	p.closed = true
	p.cond.Broadcast()
	p.mu.Unlock()
}

func (p *Pool) worker() {
	for {
		q, ctx, item, ok := p.take()
		if !ok {
			return
		}
		err := item(ctx)
		p.complete(q, err)
	}
}

func (p *Pool) take() (*queue, context.Context, Work, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for {
		if p.closed {
			return nil, nil, nil, false
		}
		for _, q := range p.queues {
			if q.paused || q.done || q.next >= len(q.items) {
				continue
			}
			if p.running >= p.maxSize {
				break
			}
			item := q.items[q.next]
			q.next++
			q.running++
			p.running++
			return q, q.ctx, item, true
		}
		p.cond.Wait()
	}
}

func (p *Pool) complete(q *queue, err error) {
	p.mu.Lock()
	q.running--
	p.running--
	if err != nil && !q.failed {
		q.failed = true
		q.err = err
		q.done = true
	}
	p.cond.Broadcast()
	p.mu.Unlock()
}
