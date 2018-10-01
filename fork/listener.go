// +build !windows
// +build !js
// +build !appengine

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
	case "tcp", "tcp4":
		tcpaddr, err1 := net.ResolveTCPAddr(network, address)
		if tcpaddr != nil && tcpaddr.IP == nil {
			tcpaddr.IP = net.IPv4zero
		}
		addr, err = tcpaddr, err1
	case "tcp6":
		tcpaddr, err1 := net.ResolveTCPAddr(network, address)
		if tcpaddr != nil && tcpaddr.IP == nil {
			tcpaddr.IP = net.IPv6zero
		}
		addr, err = tcpaddr, err1
	case "unix", "unixpacket":
		addr, err = net.ResolveUnixAddr(network, address)
	default:
		err = fmt.Errorf("network `%s' unsupported", network)
	}
	if err != nil {
		return nil, err
	}
	filename := filename(addr)
	var ln net.Listener
	if fd, ok := inheritedFDs[filename]; ok {
		file := os.NewFile(fd, filename)
		if file == nil {
			return nil, fmt.Errorf("unable to create listener file %s", filename)
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
		// envListeners[f.Name()] = i + 3
		envListeners[filename(ln.Addr())] = i + 3
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

// same style of net/fd_unix.go: *netFD.name()
func filename(laddr net.Addr) string {
	return laddr.Network() + ":" + laddr.String() + "->"
}

type ReloadablePacketConn interface {
	net.PacketConn
	File() (*os.File, error)
}

func ListenPacket(network, address string) (c ReloadablePacketConn, err error) {
	var addr net.Addr
	switch network {
	case "udp", "udp4":
		laddr, err := net.ResolveUDPAddr(network, address)
		if err != nil {
			return nil, err
		}
		if laddr != nil && laddr.IP == nil {
			laddr.IP = net.IPv4zero
		}
		addr = laddr
	case "udp6":
		laddr, err := net.ResolveUDPAddr(network, address)
		if err != nil {
			return nil, err
		}
		if laddr != nil && laddr.IP == nil {
			laddr.IP = net.IPv6zero
		}
		addr = laddr
	case "unix", "unixgram":
		addr, err = net.ResolveUnixAddr(network, address)
	default:
		err = fmt.Errorf("network `%s' unsupported", network)
	}

	filename := filename(addr)
	var conn net.PacketConn
	if fd, ok := inheritedFDs[filename]; ok {
		file := os.NewFile(fd, filename)
		if file == nil {
			return nil, fmt.Errorf("unable to create conn file %s", filename)
		}
		conn, err = net.FilePacketConn(file)
		if err != nil {
			return nil, err
		}
		defer file.Close()
	} else {
		conn, err = net.ListenPacket(network, address)
		if err != nil {
			return
		}
	}
	c = conn.(ReloadablePacketConn)
	return
}
