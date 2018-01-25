package sigutil

import (
	"os"
	"os/signal"
	"syscall"
)

const (
	SIGABRT = syscall.SIGABRT
	SIGALRM = syscall.SIGALRM
	SIGBUS  = syscall.SIGBUS
	SIGCHLD = syscall.SIGCHLD
	SIGCONT = syscall.SIGCONT
	SIGFPE  = syscall.SIGFPE
	SIGHUP  = syscall.SIGHUP
	SIGILL  = syscall.SIGILL
	SIGINT  = syscall.SIGINT
	SIGIO   = syscall.SIGIO
	SIGIOT  = syscall.SIGIOT
	SIGKILL = syscall.SIGKILL
	SIGPIPE = syscall.SIGPIPE
	SIGQUIT = syscall.SIGQUIT
	SIGSEGV = syscall.SIGSEGV
	SIGSTOP = syscall.SIGSTOP
	SIGSYS  = syscall.SIGSYS
	SIGTERM = syscall.SIGTERM
	SIGTRAP = syscall.SIGTRAP
	SIGTSTP = syscall.SIGTSTP
	SIGTTIN = syscall.SIGTTIN
	SIGTTOU = syscall.SIGTTOU
	SIGURG  = syscall.SIGURG
	SIGUSR1 = syscall.SIGUSR1
	SIGUSR2 = syscall.SIGUSR2
)

type Signal = os.Signal

func Trap(cb func(Signal), signals ...Signal) {
	s := make(chan Signal, 1)
	signal.Notify(s, signals...)
	cb(<-s)
	signal.Stop(s)
	close(s)
}
