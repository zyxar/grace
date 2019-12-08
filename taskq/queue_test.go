package taskq_test

import (
	"context"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zyxar/grace/taskq"
)

var workerCount int = runtime.NumCPU()

func BenchmarkQueue(b *testing.B) {
	ctx := context.Background()
	const total = 1000000
	var bench = func(pb *testing.B, capacity, count int) {
		a := uint64(0)
		q := taskq.New(ctx, capacity, count)
		pb.ResetTimer()
		for i := 0; i < total; i++ {
			q.Push(ctx, taskq.TaskFunc(func() {
				atomic.AddUint64(&a, 1)
			}))
		}
		q.Close()
		q.Wait()
	}

	b.Run("10+10", func(pb *testing.B) { bench(pb, 10, 10) })
	b.Run("100+10", func(pb *testing.B) { bench(pb, 100, 10) })
	b.Run("1000+10", func(pb *testing.B) { bench(pb, 1000, 10) })
	b.Run("10000+10", func(pb *testing.B) { bench(pb, 10000, 10) })
	b.Run("100000+10", func(pb *testing.B) { bench(pb, 100000, 10) })

	b.Run("100+100", func(pb *testing.B) { bench(pb, 100, 100) })
	b.Run("1000+100", func(pb *testing.B) { bench(pb, 1000, 100) })
	b.Run("10000+100", func(pb *testing.B) { bench(pb, 10000, 100) })
	b.Run("100000+100", func(pb *testing.B) { bench(pb, 100000, 100) })
}

func BenchmarkQueuePush(b *testing.B) {
	ctx := context.Background()
	var bench = func(pb *testing.B, capacity, count int) {
		a := uint64(0)
		q := taskq.New(ctx, capacity, count)
		pb.ResetTimer()
		for i := 0; i < pb.N; i++ {
			q.Push(ctx, taskq.TaskFunc(func() {
				atomic.AddUint64(&a, 1)
			}))
		}
		pb.StopTimer()
		q.Close()
		q.Wait()
	}

	b.Run("10+10", func(pb *testing.B) { bench(pb, 10, 10) })
	b.Run("100+10", func(pb *testing.B) { bench(pb, 100, 10) })
	b.Run("1000+10", func(pb *testing.B) { bench(pb, 1000, 10) })
	b.Run("10000+10", func(pb *testing.B) { bench(pb, 10000, 10) })
	b.Run("100000+10", func(pb *testing.B) { bench(pb, 100000, 10) })

	b.Run("100+"+strconv.Itoa(workerCount), func(pb *testing.B) { bench(pb, 100, workerCount) })
	b.Run("1000+"+strconv.Itoa(workerCount), func(pb *testing.B) { bench(pb, 1000, workerCount) })
	b.Run("10000+"+strconv.Itoa(workerCount), func(pb *testing.B) { bench(pb, 10000, workerCount) })
	b.Run("100000+"+strconv.Itoa(workerCount), func(pb *testing.B) { bench(pb, 100000, workerCount) })
}

func TestEmptyQueue(t *testing.T) {
	ctx := context.Background()
	t.Parallel()
	q := taskq.New(ctx, 1000, workerCount)
	if q.IsDone() {
		t.Error("queue done")
	}
	q.Close()
	q.Cancel()
	q.Close()
	q.Cancel()
	q.Close()
	q.Wait()
	if !q.IsDone() {
		t.Error("queue undone")
	}
}

func TestQueueNoPanicOnCancel(t *testing.T) {
	// t.Parallel()
	ctx := context.Background()
	q := taskq.New(ctx, workerCount, workerCount)
	var wg sync.WaitGroup
	for i := 0; i < 10000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				q.Push(ctx, taskq.TaskFunc(func() {
					time.Sleep(time.Millisecond)
				}))
			}
		}()
	}

	time.Sleep(time.Millisecond * 10)
	q.Cancel()

	q.Wait()
	wg.Wait()
}

func TestQueueNoPanicOnClose(t *testing.T) {
	// t.Parallel()
	ctx := context.Background()
	q := taskq.New(ctx, workerCount, workerCount)
	var wg sync.WaitGroup
	for i := 0; i < 10000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				q.Push(ctx, taskq.TaskFunc(func() {
					time.Sleep(time.Millisecond)
				}))
			}
		}()
	}

	time.Sleep(time.Millisecond * 10)
	q.Close()

	q.Wait()
	wg.Wait()
}

func TestQueueFunction(t *testing.T) {
	t.Parallel()
	const total = 1000000
	a := uint64(0)
	ctx := context.Background()
	q := taskq.New(ctx, 10, workerCount)
	for i := 0; i < total; i++ {
		q.Push(ctx, taskq.TaskFunc(func() {
			atomic.AddUint64(&a, 1)
		}))
	}
	q.Close()
	q.Wait()
	for i := 0; i < workerCount; i++ {
		go q.Close()
	}
	if v := atomic.LoadUint64(&a); v != total {
		t.Errorf("value incorrect, expected=%d,got=%d", total, v)
	}
}

func TestQueueClose(t *testing.T) {
	t.Parallel()
	const total = 1000000
	a := uint64(0)
	ctx := context.Background()
	q := taskq.New(ctx, 100, workerCount)
	for i := 0; i < total; i++ {
		q.Push(ctx, taskq.TaskFunc(func() {
			atomic.AddUint64(&a, 1)
		}))
	}
	q.Close()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := q.Push(ctx, taskq.TaskFunc(func() {
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
	ctx := context.Background()
	q := taskq.New(ctx, 100, workerCount)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < total; i++ {
			q.Push(ctx, taskq.TaskFunc(func() {
				atomic.AddUint64(&a, 1)
			}))
		}
	}()
	q.Cancel()
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := q.Push(ctx, taskq.TaskFunc(func() {
				atomic.AddUint64(&a, 1)
			})); err != taskq.ErrQueueCancelled {
				t.Error("push should fail with cancel", err)
			}
		}()
	}
	q.Close()
	q.Wait()
	wg.Wait()
	if v := atomic.LoadUint64(&a); v >= total {
		t.Errorf("value incorrect, expected<%d,got=%d", total, v)
	}
}

func TestQueueLen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q := taskq.New(ctx, 100, workerCount)
	var wg, wg1, wg2 sync.WaitGroup
	wg.Add(1)
	wg1.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		if err := q.Push(ctx, taskq.TaskFunc(func() {
			wg1.Done()
			wg.Wait()
		})); err != nil {
			t.Error(err.Error())
		}
	}
	wg1.Wait()
	if l := q.Len(); l != 0 {
		t.Errorf("queue len=%v", l)
	}
	wg2.Add(100)
	for i := 0; i < 100; i++ {
		if err := q.Push(ctx, taskq.TaskFunc(func() {
			wg2.Done()
		})); err != nil {
			t.Error(err.Error())
		}
		if l := q.Len(); l != i+1 {
			t.Errorf("[%d] queue len=%v", i, l)
		}
	}
	wg.Done()
	wg2.Wait()
	if l := q.Len(); l != 0 {
		t.Errorf("queue len=%v", l)
	}
	q.Close()
	q.Wait()
}
