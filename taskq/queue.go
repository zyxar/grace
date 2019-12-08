package taskq

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
)

var (
	// ErrQueueClosed indicates Queue is closed
	ErrQueueClosed = errors.New("task queue closed")
	// ErrQueueCancelled indicates Queue is cancelled
	ErrQueueCancelled = errors.New("task queue cancelled")
	// ErrQueueBusy indicates Queue is full
	ErrQueueBusy = errors.New("task queue busy")
	// ErrTaskTimeout indicates task is not executed in the given timeout
	ErrTaskTimeout = errors.New("task time out")
	// ErrTaskCancelled indicates task is not executed due to cancellation
	ErrTaskCancelled = errors.New("task time out")
)

// Queue is abstraction of task execution scheduling
// When Queue is created, a fixed number (given by Caller) of workers are running to consume Tasks,
// until Close() or Cancel() is called;
// if Cancel() is called, workers exit immediately after current job done;
// if Close() is called, workers exit when all the remaining tasks are done;
// however, Cancel() has higher priority: Close() after Cancel() would be ignored;
// Caller invokes Push() (blocking) or TryPush (non-blocking) to feed the Queue, and
// Wait() to wait until Queue is cleaned up after calling Close() or Cancel().
type Queue interface {
	// Close closes the Queue, which shall not accept Push any more; however the tasks in the Queue would still be Executed
	Close()
	// Cancel closes the Queue and cancels the task execution
	Cancel()
	// Push appends a task in the Queue in blocking way, unless ctx cancelled
	Push(ctx context.Context, task Task) error
	// PushExec appends a task in the Queue and waits task to be executed unless ctx cancelled
	PushExec(ctx context.Context, task Task) error
	// TryPush appends a task in the Queue in non-blocking way
	TryPush(ctx context.Context, task Task) error
	// Wait waits until all the task execution finished, or one of ctx cancelled
	Wait(ctx ...context.Context) bool
	// Len returns the number of tasks waiting in the Queue
	Len() int
	// IsDone returns whether all active workers (goroutines) have exited, e.g., after calling Close() or Cancel()
	IsDone() bool
}

type queue struct {
	noCopy    noCopy
	tchan     chan Task
	ctx       context.Context
	cancel    context.CancelFunc
	closeCh   chan struct{}
	closeOnce sync.Once
	workerNum *int64
	wg        *sync.WaitGroup
}

var _ Queue = &queue{}

var defaultWorkerCount = runtime.NumCPU()

// New creates a Queue instance with `count` workers and buffer size of `capacity`
func New(ctx context.Context, capacity, count int) Queue {
	if count <= 0 {
		count = defaultWorkerCount
	}
	tchan := make(chan Task, capacity)
	var wg sync.WaitGroup
	var workerNum = int64(count)
	ctx, cancel := context.WithCancel(ctx)
	closeCh := make(chan struct{})
	work := func(tasks <-chan Task) {
		defer atomic.AddInt64(&workerNum, -1)
		defer wg.Done()
		for {
			select {
			case <-ctx.Done(): // cancel
				return
			case <-closeCh: // close
				select {
				case <-ctx.Done():
					return
				default:
				}
				for {
					select {
					case <-ctx.Done(): // cancel
						return
					case task := <-tasks:
						task.Execute()
					default:
						return
					}
				}
			default:
			}

			select {
			case <-ctx.Done(): // cancel
				return
			case task := <-tasks:
				task.Execute()
			case <-closeCh: // close
				continue
			}
		}
	}
	for i := 0; i < count; i++ {
		wg.Add(1)
		go work(tchan)
	}

	return &queue{
		tchan:     tchan,
		ctx:       ctx,
		cancel:    cancel,
		closeCh:   closeCh,
		workerNum: &workerNum,
		wg:        &wg,
	}
}

func (q *queue) Cancel()      { q.cancel() }
func (q *queue) Close()       { q.closeOnce.Do(func() { close(q.closeCh) }) }
func (q *queue) Len() int     { return len(q.tchan) } // Len returns the number of tasks waiting in the queue.
func (q *queue) IsDone() bool { return atomic.LoadInt64(q.workerNum) == 0 }

func (q *queue) Wait(ctxs ...context.Context) bool {
	ret := make(chan bool, len(ctxs))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, c := range ctxs {
		go func() {
			select {
			case <-ctx.Done():
				return
			case <-c.Done():
				ret <- false
			}
		}()
	}

	go func() {
		q.wg.Wait()
		cancel()
		ret <- true
	}()

	return <-ret
}

func (q *queue) Push(ctx context.Context, task Task) error {
	select {
	case <-q.ctx.Done():
		return ErrQueueCancelled
	case <-q.closeCh:
		select {
		case <-q.ctx.Done():
			return ErrQueueCancelled
		default:
		}
		return ErrQueueClosed
	case <-ctx.Done():
		return ErrTaskCancelled
	default:
	}

	select {
	case <-q.ctx.Done():
		return ErrQueueCancelled
	case <-q.closeCh:
		select {
		case <-q.ctx.Done():
			return ErrQueueCancelled
		default:
		}
		return ErrQueueClosed
	case <-ctx.Done():
		return ErrTaskCancelled
	case q.tchan <- task:
		return nil
	}
}

func (q *queue) TryPush(ctx context.Context, task Task) error {
	select {
	case <-q.ctx.Done():
		return ErrQueueCancelled
	case <-q.closeCh:
		select {
		case <-q.ctx.Done():
			return ErrQueueCancelled
		default:
		}
		return ErrQueueClosed
	case <-ctx.Done():
		return ErrTaskCancelled
	default:
	}

	select {
	case <-q.ctx.Done():
		return ErrQueueCancelled
	case <-q.closeCh:
		select {
		case <-q.ctx.Done():
			return ErrQueueCancelled
		default:
		}
		return ErrQueueClosed
	case <-ctx.Done():
		return ErrTaskCancelled
	case q.tchan <- task:
		return nil
	default:
		return ErrQueueBusy
	}
}

// PushExec appends a task in the Queue and waits task to be executed unless ctx cancelled
func (q *queue) PushExec(ctx context.Context, task Task) error {
	select {
	case <-q.ctx.Done():
		return ErrQueueCancelled
	case <-q.closeCh:
		select {
		case <-q.ctx.Done():
			return ErrQueueCancelled
		default:
		}
		return ErrQueueClosed
	case <-ctx.Done():
		return ErrTaskCancelled
	default:
	}

	t := &contextTask{task: task, done: make(chan struct{})}
	var cancel context.CancelFunc
	t.ctx, cancel = context.WithCancel(ctx)
	defer cancel()

	select {
	case <-q.ctx.Done():
		return ErrQueueCancelled
	case <-q.closeCh:
		select {
		case <-q.ctx.Done():
			return ErrQueueCancelled
		default:
		}
		return ErrQueueClosed
	case <-ctx.Done():
		return ErrQueueBusy
	case q.tchan <- t:
	}

	select {
	case <-ctx.Done():
		select {
		default:
			return ErrTaskTimeout
		case <-t.done:
		}
	case <-t.done:
	}
	return nil
}
