package sig

import (
	"os/signal"
	"sync"
)

type SigHandle struct {
	s  SigChan
	wg *sync.WaitGroup
}

func Watch(fn func(Signal), signals ...Signal) *SigHandle {
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
	return &SigHandle{s, wg}
}

func (s *SigHandle) Close() {
	s.s.Close()
	s.wg.Wait()
}
