package taskq

import (
	"sync"
	"time"
)

type Task interface {
	Execute()
}

type TaskFunc func()

func (t TaskFunc) Execute() {
	t()
}

type Queue struct {
	queue           chan Task
	wg              *sync.WaitGroup
	cancel, closing schan
}

type schan chan struct{}

func New(capacity, count int) *Queue {
	q := make(chan Task, capacity)
	cancel := make(schan)
	closing := make(schan)
	var wg sync.WaitGroup
	work := func(tasks <-chan Task) {
		defer wg.Done()
		for {
			select {
			case <-closing:
				return
			case <-cancel:
				return
			default:
			}

			select {
			case <-closing: // below clause may block; this is an `exit' point for worker routine
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
	return &Queue{q, &wg, cancel, closing}
}

func (q *Queue) Push(task Task) {
	select {
	case <-q.closing:
		return
	case q.queue <- task:
	}
}

func (q *Queue) TryPush(task Task) bool {
	select {
	case <-q.closing:
		return false
	case q.queue <- task:
		return true
	default:
		return false
	}
}

func (q *Queue) Wait() {
	q.wg.Wait()
}

func (q *Queue) Cancel() {
	close(q.cancel)
}

func (q *Queue) CancelAndWait() {
	close(q.cancel)
	q.wg.Wait()
}

func (q *Queue) Close() {
	close(q.closing)
}

func (q *Queue) CloseAndWait() {
	close(q.closing)
	q.wg.Wait()
}

func (q *Queue) CloseAndWaitTimeout(d time.Duration) bool {
	ch := make(schan)
	close(q.closing)
	go func() {
		q.wg.Wait()
		close(ch)
	}()
	select {
	case <-ch:
		return true
	case <-time.After(d):
		return false
	}
}
