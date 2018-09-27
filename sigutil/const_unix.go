// +build linux darwin dragonfly freebsd netbsd openbsd
// +build !appengine

package sigutil

import "syscall"

const (
	SIGCHLD = syscall.SIGCHLD
	SIGCONT = syscall.SIGCONT
	SIGSTOP = syscall.SIGSTOP
	SIGSYS  = syscall.SIGSYS
	SIGTSTP = syscall.SIGTSTP
	SIGURG  = syscall.SIGURG
	SIGUSR1 = syscall.SIGUSR1
	SIGUSR2 = syscall.SIGUSR2
)
