package taskq_test

import (
	"runtime"
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
	q.CloseAndWait()
}

func TestQueueTryPushClose(t *testing.T) {
	t.Parallel()
	a := uint64(0)
	q := taskq.New(1000, workerCount)
	for i := 0; i < 10000; i++ {
		q.TryPush(taskq.TaskFunc(func() {
			atomic.AddUint64(&a, 1)
		}))
	}
	q.CloseAndWait()
}

func TestQueuePushClose(t *testing.T) {
	t.Parallel()
	a := uint64(0)
	q := taskq.New(1000, workerCount)
	for i := 0; i < 10000; i++ {
		q.Push(taskq.TaskFunc(func() {
			atomic.AddUint64(&a, 1)
		}))
	}
	q.CloseAndWait()
}
