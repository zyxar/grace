package fork

import (
	"fmt"
	"net"
	"os"
	"syscall"
)

const envInheritListener = `INHERIT_LISTENER`
const envInheritListenerVal = "true"
const lfd uintptr = 3 // 0=>stdin, 1=>stdout, 2=>stderr

// ReloadableListener can be reloaded when forking, typically *net.TCPListener or *net.UnixListener
type ReloadableListener interface {
	net.Listener
	File() (*os.File, error)
}

// Listen reloads inherited listener or creates a new one; only TCP & Unix network supported.
// Listen should be called instead of normal `net.Listen` calls.
func Listen(network, address string) (l ReloadableListener, err error) {
	var addr net.Addr
	switch network {
	case "tcp", "tcp4", "tcp6":
		addr, err = net.ResolveTCPAddr(network, address)
	case "unix", "unixgram", "unixpacket":
		addr, err = net.ResolveUnixAddr(network, address)
	default:
		err = fmt.Errorf("network %q unsupported", network)
	}
	if err != nil {
		return nil, err
	}
	var ln net.Listener
	if os.Getenv(envInheritListener) == envInheritListenerVal {
		filename := fmt.Sprintf("%s:%s", addr.Network(), addr.String())
		file := os.NewFile(lfd, filename)
		if file == nil {
			return nil, fmt.Errorf("unable to create listener file %v", filename)
		}
		ln, err = net.FileListener(file)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		if ln.Addr().Network() != addr.Network() || ln.Addr().String() != addr.String() {
			return nil, fmt.Errorf("listener mismatch %s:%s != %s:%s",
				ln.Addr().Network(), ln.Addr().String(), addr.Network(), addr.String())
		}
	} else {
		ln, err = net.Listen(network, address)
		if err != nil {
			return
		}
	}
	l = ln.(ReloadableListener)
	return
}

// Reload forks and executes the same program (identical file path),
// sending listener fd to be inherited by child process
// Reload return child process id; or 0 and error upon any failures.
func Reload(ln ReloadableListener) (int, error) {
	bin, err := os.Executable()
	if err != nil {
		return -1, err
	}
	wd, err := os.Getwd()
	if err != nil {
		return -1, err
	}
	if err = os.Setenv(envInheritListener, envInheritListenerVal); err != nil {
		return -1, err
	}

	f, err := ln.File()
	if err != nil {
		return -1, err
	}

	return syscall.ForkExec(bin, os.Args, &syscall.ProcAttr{
		Dir:   wd,
		Env:   os.Environ(),
		Files: []uintptr{os.Stdin.Fd(), os.Stdout.Fd(), os.Stderr.Fd(), f.Fd()},
	})
}
