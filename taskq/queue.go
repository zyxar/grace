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
	closeFn         func()
}

type schan chan struct{}

func New(capacity, count int) *Queue {
	q := make(chan Task, capacity)
	cancel := make(schan)
	closing := make(schan)
	sigClose := make(schan, 1)
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
			case <-closing:
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
	go func() { // moderator
		<-sigClose
		close(closing)
	}()
	return &Queue{q, &wg, cancel, closing, func() {
		select {
		case sigClose <- struct{}{}:
		default:
		}
	}}
}

func (q *Queue) Push(task Task) {
	select {
	case <-q.closing:
		return
	default:
	}
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
	default:
	}
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
	q.closeFn()
}

func (q *Queue) CloseAndWait() {
	q.closeFn()
	q.wg.Wait()
}

func (q *Queue) CloseAndWaitTimeout(d time.Duration) bool {
	q.closeFn()
	ch := make(schan)
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
