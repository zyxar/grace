package taskq

import "context"

// Task is excution unit in Queue
type Task interface{ Execute() }

// TaskFunc is bulit-in implementation of Task
type TaskFunc func()

// Execute implements Task interface
func (t TaskFunc) Execute() { t() }

type contextTask struct {
	task Task
	ctx  context.Context
	done chan struct{}
}

func (c *contextTask) Execute() {
	select {
	case <-c.ctx.Done():
		return
	default:
	}
	c.task.Execute()
	close(c.done)
}
