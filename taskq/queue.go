package taskq

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrQueueClosed   = errors.New("task queue closed")
	ErrQueueCanceled = errors.New("task queue canceled")
	ErrQueueBusy     = errors.New("task queue busy")
)

type Task interface {
	Execute()
}

type TaskFunc func()

func (t TaskFunc) Execute() { t() }

type Queue struct {
	queue  chan Task
	cancel schan
	wg     *sync.WaitGroup
	mutex  *sync.RWMutex
	once   *sync.Once
	closed bool // protected by mutex
}

type schan chan struct{}

func New(capacity, count int) *Queue {
	q := make(chan Task, capacity)
	var wg sync.WaitGroup
	cancel := make(schan)
	work := func(tasks <-chan Task) {
		defer wg.Done()
		for {
			select {
			case <-cancel:
				return
			default:
			}

			select {
			case <-cancel:
				return
			case task, ok := <-tasks:
				if !ok {
					return
				}
				task.Execute()
			}
		}
	}
	for i := 0; i < count; i++ {
		wg.Add(1)
		go work(q)
	}
	return &Queue{q, cancel, &wg, &sync.RWMutex{}, &sync.Once{}, false}
}

func (q *Queue) close() {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.closed = true
	close(q.queue)
}

func (q *Queue) Close() {
	q.once.Do(q.close)
}

func (q *Queue) Cancel() {
	q.once.Do(func() {
		close(q.cancel)
	})
}

func (q *Queue) Wait() {
	q.wg.Wait()
}

func (q *Queue) WaitTimeout(d time.Duration) bool {
	end := make(schan)
	go func() {
		q.wg.Wait()
		close(end)
	}()
	select {
	case <-end:
		return true
	case <-time.After(d):
		return false
	}
}

func (q *Queue) Push(task Task) error {
	select {
	case <-q.cancel:
		return ErrQueueCanceled
	default:
	}
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	if q.closed {
		return ErrQueueClosed
	}
	q.queue <- task
	return nil
}

func (q *Queue) TryPush(task Task) error {
	select {
	case <-q.cancel:
		return ErrQueueCanceled
	default:
	}
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	if q.closed {
		return ErrQueueClosed
	}
	select {
	case q.queue <- task:
		return nil
	default:
		return ErrQueueBusy
	}
}
