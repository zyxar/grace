package sigutil

import (
	"os/signal"
	"sync"
)

type Handle struct {
	sc chan Signal
	wg *sync.WaitGroup
}

func Watch(fn func(Signal), signals ...Signal) *Handle {
	sc := make(chan Signal, 1)
	wg := &sync.WaitGroup{}
	signal.Notify(sc, signals...)
	wg.Add(1)
	go func() {
		for c := range sc {
			fn(c)
		}
		wg.Done()
	}()
	return &Handle{sc, wg}
}

func (s *Handle) Close() {
	signal.Stop(s.sc)
	close(s.sc)
	s.wg.Wait()
}
