package taskq_test

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/zyxar/grace/taskq"
)

var workerCount int

func init() {
	workerCount = runtime.NumCPU()
}

func TestEmptyQueue(t *testing.T) {
	t.Parallel()
	q := taskq.New(1000, workerCount)
	q.Close()
	q.Cancel()
	q.Close()
	q.Wait()
}

func TestQueueFunction(t *testing.T) {
	t.Parallel()
	const total = 1000000
	a := uint64(0)
	q := taskq.New(10, workerCount)
	for i := 0; i < total; i++ {
		q.Push(taskq.TaskFunc(func() {
			atomic.AddUint64(&a, 1)
		}))
	}
	for i := 0; i < 10; i++ {
		go q.Close()
	}
	q.Wait()
	if v := atomic.LoadUint64(&a); v != total {
		t.Errorf("value incorrect, expected=%d,got=%d", total, v)
	}
}

func TestQueueClose(t *testing.T) {
	t.Parallel()
	const total = 1000000
	a := uint64(0)
	q := taskq.New(100, workerCount)
	for i := 0; i < total; i++ {
		q.Push(taskq.TaskFunc(func() {
			atomic.AddUint64(&a, 1)
		}))
	}
	q.Close()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := q.Push(taskq.TaskFunc(func() {
				atomic.AddUint64(&a, 1)
			})); err != taskq.ErrQueueClosed {
				t.Error("push should fail with close")
			}
		}()
	}
	wg.Wait()
	q.Wait()
	if v := atomic.LoadUint64(&a); v != total {
		t.Errorf("value incorrect, expected=%d,got=%d", total, v)
	}
}
func TestQueueCancel(t *testing.T) {
	t.Parallel()
	const total = 1000000
	a := uint64(0)
	q := taskq.New(10, workerCount)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < total; i++ {
			q.Push(taskq.TaskFunc(func() {
				atomic.AddUint64(&a, 1)
			}))
		}
	}()
	q.Cancel()
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := q.Push(taskq.TaskFunc(func() {
				atomic.AddUint64(&a, 1)
			})); err != taskq.ErrQueueCanceled {
				t.Error("push should fail with cancel")
			}
		}()
	}
	q.Close()
	wg.Wait()
	q.Wait()
	if v := atomic.LoadUint64(&a); v >= total {
		t.Errorf("value incorrect, expected<%d,got=%d", total, v)
	}
}
