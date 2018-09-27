package sigutil

import (
	"os"
	"os/signal"
	"syscall"
)

const (
	SIGINT  = syscall.SIGINT
	SIGKILL = syscall.SIGKILL
	SIGQUIT = syscall.SIGQUIT
	SIGTRAP = syscall.SIGTRAP
)

type Signal = os.Signal

func Trap(cb func(Signal), signals ...Signal) {
	s := make(chan Signal, 1)
	signal.Notify(s, signals...)
	cb(<-s)
	signal.Stop(s)
	close(s)
}
