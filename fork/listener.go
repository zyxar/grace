package fork

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"syscall"
)

const envInheritListener = `FORK_INHERIT_LISTENER`

// ReloadableListener can be reloaded when forking, typically *net.TCPListener or *net.UnixListener
type ReloadableListener interface {
	net.Listener
	File() (*os.File, error)
}

var inheritedFDs = map[string]uintptr{} // readonly after init

func init() {
	if env := os.Getenv(envInheritListener); env != "" {
		json.Unmarshal([]byte(env), &inheritedFDs)
	}
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
	filename := fmt.Sprintf("%s:%s", addr.Network(), addr.String())
	var ln net.Listener
	if fd, ok := inheritedFDs[filename]; ok {
		file := os.NewFile(fd, filename)
		if file == nil {
			return nil, fmt.Errorf("unable to create listener file %v", filename)
		}
		ln, err = net.FileListener(file)
		if err != nil {
			return nil, err
		}
		defer file.Close()
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
func Reload(listeners ...ReloadableListener) (int, error) {
	bin, err := os.Executable()
	if err != nil {
		return -1, err
	}
	wd, err := os.Getwd()
	if err != nil {
		return -1, err
	}
	envListeners := make(map[string]int)
	files := make([]uintptr, 3, 3+len(listeners))
	files[0], files[1], files[2] = os.Stdin.Fd(), os.Stdout.Fd(), os.Stderr.Fd()
	for i, ln := range listeners {
		f, err := ln.File()
		if err != nil {
			return -1, err
		}
		files = append(files, f.Fd())
		filename := fmt.Sprintf("%s:%s", ln.Addr().Network(), ln.Addr().String())
		envListeners[filename] = i + 3
	}
	b, err := json.Marshal(envListeners)
	if err != nil {
		return -1, err
	}
	if err = os.Setenv(envInheritListener, string(b)); err != nil {
		return -1, err
	}

	return syscall.ForkExec(bin, os.Args, &syscall.ProcAttr{
		Dir:   wd,
		Env:   os.Environ(),
		Files: files,
	})
}
