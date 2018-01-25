package sigutil

import (
	"os/signal"
	"sync"
)

type Handle struct {
	s  SigChan
	wg *sync.WaitGroup
}

func Watch(fn func(Signal), signals ...Signal) *Handle {
	s := make(SigChan, 1)
	wg := &sync.WaitGroup{}
	signal.Notify(s, signals...)
	wg.Add(1)
	go func() {
	loop:
		for {
			select {
			case c, ok := <-s:
				if ok {
					fn(c)
				} else {
					break loop
				}
			}
		}
		wg.Done()
	}()
	return &Handle{s, wg}
}

func (s *Handle) Close() {
	s.s.Close()
	s.wg.Wait()
}
