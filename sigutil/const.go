// +build !js

package sigutil

import "syscall"

const (
	SIGABRT = syscall.SIGABRT
	SIGALRM = syscall.SIGALRM
	SIGBUS  = syscall.SIGBUS
	SIGFPE  = syscall.SIGFPE
	SIGHUP  = syscall.SIGHUP
	SIGILL  = syscall.SIGILL
	SIGPIPE = syscall.SIGPIPE
	SIGSEGV = syscall.SIGSEGV
	SIGTERM = syscall.SIGTERM
)
