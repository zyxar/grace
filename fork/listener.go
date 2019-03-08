// +build !windows
// +build !js
// +build !appengine

package fork

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strconv"
	"sync"
	"syscall"
	"time"
)

const envInheritListener = `FORK_INHERIT_LISTENER`
const magicNumber = 3 // where listener FDs start from
const pipeFilename = `FORK_PIPE`

// Reloadable can be reloaded when forking (by calling Reload()), typically ReloadableListener or ReloadablePacketConn
// Reloadable provides underlying file descriptor through calling File(), which can be used in constructing
// net.Listener or net.PacketConn via net.FileListener() or net.FilePacketConn(), respectively.
type Reloadable interface {
	File() (*os.File, error)
}

// ReloadableListener can be reloaded when forking, typically *net.TCPListener or *net.UnixListener
type ReloadableListener interface {
	net.Listener
	Reloadable
}

// ReloadablePacketConn can be reloaded when forking, typically *net.UDPConn or *net.UnixConn
type ReloadablePacketConn interface {
	net.PacketConn
	Reloadable
}

var inheritedFDs = map[string]uintptr{} // readonly after init

func init() {
	if env := os.Getenv(envInheritListener); env != "" {
		json.Unmarshal([]byte(env), &inheritedFDs)
	}
}

func resolveAddr(network, address string) (addr net.Addr, err error) {
	switch network {
	case "unix", "unixpacket", "unixgram":
		return net.ResolveUnixAddr(network, address)
	default:
	}

	ip, port, err := func(network, address string) (iaddr *net.IPAddr, portnum int, err error) {
		switch network {
		case "tcp", "tcp4", "tcp6", "udp", "udp4", "udp6":
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return iaddr, portnum, err
			}
			portnum, err = net.LookupPort(network, port)
			if err != nil {
				return iaddr, portnum, err
			}
			iaddr, err = net.ResolveIPAddr("ip", host)
			if err != nil {
				return iaddr, portnum, err
			}
		default:
			err = fmt.Errorf("network `%s' unsupported", network)
		}
		return
	}(network, address)
	if err != nil {
		return nil, err
	}
	if ip.IP == nil {
		if network != "" && network[len(network)-1] == '6' {
			ip.IP = net.IPv6zero
		} else {
			ip.IP = net.IPv4zero
		}
	}
	switch network {
	case "tcp", "tcp4", "tcp6":
		addr = &net.TCPAddr{IP: ip.IP, Port: port, Zone: ip.Zone}
	case "udp", "udp4", "udp6":
		addr = &net.UDPAddr{IP: ip.IP, Port: port, Zone: ip.Zone}
	}
	return
}

// Listen reloads inherited listener or creates a new one; only TCP & Unix network supported.
// Listen should be called instead of normal `net.Listen` calls.
func Listen(network, address string) (l ReloadableListener, err error) {
	addr, err := resolveAddr(network, address)
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
		defer file.Close()
		ln, err = net.FileListener(file)
		if err != nil {
			return nil, err
		}
		if l, ok = ln.(ReloadableListener); !ok {
			return nil, fmt.Errorf("listener %s not reloadable", filename)
		}
		return l, nil
	}
	switch network {
	case "tcp", "tcp4":
		l, err = net.ListenTCP("tcp4", addr.(*net.TCPAddr))
	case "tcp6":
		l, err = net.ListenTCP("tcp6", addr.(*net.TCPAddr))
	case "unix", "unixpacket":
		l, err = net.ListenUnix(network, addr.(*net.UnixAddr))
	}
	return
}

// Reload forks and executes the same program (identical file path),
// sending listener fd to be inherited by child process
// Reload return child process id; or -1 and error upon any failures.
// Child process should call SignalParent() to let parent process exit.
func Reload(reloadables ...Reloadable) (int, error) {
	bin, err := os.Executable()
	if err != nil {
		return -1, err
	}
	wd, err := os.Getwd()
	if err != nil {
		return -1, err
	}
	r, w, err := os.Pipe()
	if err != nil {
		return -1, err
	}
	envListeners := make(map[string]int)
	files := make([]uintptr, magicNumber, magicNumber+len(reloadables)+1)
	files[0], files[1], files[2] = os.Stdin.Fd(), os.Stdout.Fd(), os.Stderr.Fd()
	for i, re := range reloadables {
		f, err := re.File()
		if err != nil {
			return -1, err
		}
		defer f.Close()
		switch r := re.(type) {
		case ReloadableListener:
			envListeners[filename(r.Addr())] = i + magicNumber
		case ReloadablePacketConn:
			envListeners[filename(r.LocalAddr())] = i + magicNumber
		default:
			envListeners[f.Name()] = i + magicNumber
		}
		files = append(files, f.Fd())
	}

	files = append(files, w.Fd())
	envListeners[pipeFilename] = len(files) - 1

	b, err := json.Marshal(envListeners)
	if err != nil {
		return -1, err
	}
	if err = os.Setenv(envInheritListener, string(b)); err != nil {
		return -1, err
	}

	pid, err := syscall.ForkExec(bin, os.Args, &syscall.ProcAttr{
		Dir:   wd,
		Env:   os.Environ(),
		Files: files,
	})
	if err != nil {
		return pid, err
	}
	// close pipe
	if err := w.Close(); err != nil {
		return pid, err
	}
	// waiting for child's signal
	data, err := ioutil.ReadAll(r)
	if len(data) == 0 {
		var status syscall.WaitStatus
		var rusage syscall.Rusage
		_, e := syscall.Wait4(pid, &status, 0, &rusage)
		if e != nil {
			return pid, os.NewSyscallError("wait", e)
		}
		return pid, fmt.Errorf("child <%d> status: %s, error: %v", pid, getChildStatus(status), err)
	}
	return pid, nil
}

var signalParentOnce sync.Once

// SignalParent tells parent that child process has finished initialization
// and parent process can then leave
func SignalParent() (err error) {
	signalParentOnce.Do(func() {
		if fd, ok := inheritedFDs[pipeFilename]; ok {
			pipe := os.NewFile(fd, "")
			_, err = pipe.Write([]byte("OK"))
			pipe.Close()
		}
	})
	return
}

func getChildStatus(status syscall.WaitStatus) string { // stripped from exec_posix.go
	res := ""
	switch {
	case status.Exited():
		res = "exit status " + strconv.Itoa(status.ExitStatus())
	case status.Signaled():
		res = "signal: " + status.Signal().String()
	case status.Stopped():
		res = "stop signal: " + status.StopSignal().String()
		if status.StopSignal() == syscall.SIGTRAP && status.TrapCause() != 0 {
			res += " (trap " + strconv.Itoa(status.TrapCause()) + ")"
		}
	case status.Continued():
		res = "continued"
	}
	if status.CoreDump() {
		res += " (core dumped)"
	}
	return res
}

// same style of net/fd_unix.go: *netFD.name()
func filename(laddr net.Addr) string {
	return laddr.Network() + ":" + laddr.String() + "->"
}

// ListenPacket reloads inherited net.PacketConn or creates a new one; only UDP & Unix(gram) network supported.
// ListenPacket should be called instead of normal `net.ListenPacket` calls.
func ListenPacket(network, address string) (c ReloadablePacketConn, err error) {
	addr, err := resolveAddr(network, address)
	if err != nil {
		return nil, err
	}
	filename := filename(addr)
	var conn net.PacketConn
	if fd, ok := inheritedFDs[filename]; ok {
		file := os.NewFile(fd, filename)
		if file == nil {
			return nil, fmt.Errorf("unable to create conn file %s", filename)
		}
		defer file.Close()
		conn, err = net.FilePacketConn(file)
		if err != nil {
			return nil, err
		}
		if c, ok = conn.(ReloadablePacketConn); !ok {
			return nil, fmt.Errorf("packet conn %s not reloadable", filename)
		}
		return c, nil

	}
	switch network {
	case "udp", "udp4":
		c, err = net.ListenUDP("udp4", addr.(*net.UDPAddr))
	case "udp6":
		c, err = net.ListenUDP("udp6", addr.(*net.UDPAddr))
	case "unix", "unixgram":
		c, err = net.ListenUnixgram("unixgram", addr.(*net.UnixAddr))
	}
	return
}

func TCPKeepAlive(ln net.Listener, period time.Duration) (*tcpKeepAliveListener, error) {
	if l, ok := ln.(*net.TCPListener); ok {
		return &tcpKeepAliveListener{l, period}, nil
	}
	return nil, fmt.Errorf("%T is not a *net.TCPListener", ln)
}

// tcpKeepAliveListener sets TCP keep-alive timeouts on accepted connections.
// derived from stdlib
type tcpKeepAliveListener struct {
	*net.TCPListener
	period time.Duration
}

func (ln tcpKeepAliveListener) Accept() (net.Conn, error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return nil, err
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(ln.period)
	return tc, nil
}
