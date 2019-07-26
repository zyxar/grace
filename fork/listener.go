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
	"runtime"
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

var reloadables []Reloadable
var reloadableMutex sync.RWMutex

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
			err = net.UnknownNetworkError(network)
		}
		return
	}(network, address)
	if err != nil {
		return nil, err
	}
	if ip.IP == nil {
		switch {
		case network[len(network)-1] == '4':
			ip.IP = net.IPv4zero
		case network[len(network)-1] == '6',
			supportsIPv4map(),
			!supportsIPv4():
			ip.IP = net.IPv6unspecified
		default:
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
	} else {
		switch network {
		case "tcp", "tcp4", "tcp6":
			l, err = net.ListenTCP(network, addr.(*net.TCPAddr))
		case "unix", "unixpacket":
			l, err = net.ListenUnix(network, addr.(*net.UnixAddr))
		}
	}
	reloadableMutex.Lock()
	reloadables = append(reloadables, l)
	reloadableMutex.Unlock()
	return
}

// ReloadAll forks and executes the same program (identical file path),
// Unlike Reload(), ReloadAll() uses internally maintained listener list.
func ReloadAll() (int, error) {
	reloadableMutex.RLock()
	listeners := make([]Reloadable, len(reloadables))
	copy(listeners, reloadables)
	reloadableMutex.RUnlock()
	return Reload(listeners...)
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
	case "udp", "udp4", "udp6":
		c, err = net.ListenUDP(network, addr.(*net.UDPAddr))
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

// below copied from ipsock_posix.go in stdlib; then adapted

type ipStackCapabilities struct {
	sync.Once             // guards following
	ipv4Enabled           bool
	ipv6Enabled           bool
	ipv4MappedIPv6Enabled bool
}

var ipStackCaps ipStackCapabilities

// supportsIPv4 reports whether the platform supports IPv4 networking
// functionality.
func supportsIPv4() bool {
	ipStackCaps.Once.Do(ipStackCaps.probe)
	return ipStackCaps.ipv4Enabled
}

// supportsIPv6 reports whether the platform supports IPv6 networking
// functionality.
func supportsIPv6() bool {
	ipStackCaps.Once.Do(ipStackCaps.probe)
	return ipStackCaps.ipv6Enabled
}

// supportsIPv4map reports whether the platform supports mapping an
// IPv4 address inside an IPv6 address at transport layer
// protocols. See RFC 4291, RFC 4038 and RFC 3493.
func supportsIPv4map() bool {
	ipStackCaps.Once.Do(ipStackCaps.probe)
	return ipStackCaps.ipv4MappedIPv6Enabled
}

func (p *ipStackCapabilities) probe() {
	s, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
	switch err {
	case syscall.EAFNOSUPPORT, syscall.EPROTONOSUPPORT:
	case nil:
		syscall.Close(s)
		p.ipv4Enabled = true
	}
	var probes = []struct {
		laddr net.TCPAddr
		value int
	}{
		// IPv6 communication capability
		{laddr: net.TCPAddr{IP: net.ParseIP("::1")}, value: 1},
		// IPv4-mapped IPv6 address communication capability
		{laddr: net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)}, value: 0},
	}
	switch runtime.GOOS {
	case "dragonfly", "openbsd":
		// The latest DragonFly BSD and OpenBSD kernels don't
		// support IPV6_V6ONLY=0. They always return an error
		// and we don't need to probe the capability.
		probes = probes[:1]
	}
	for i := range probes {
		s, err := syscall.Socket(syscall.AF_INET6, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
		if err != nil {
			continue
		}
		defer syscall.Close(s)
		syscall.SetsockoptInt(s, syscall.IPPROTO_IPV6, syscall.IPV6_V6ONLY, probes[i].value)
		sa, err := sockaddr(&probes[i].laddr, syscall.AF_INET6)
		if err != nil {
			continue
		}
		if err := syscall.Bind(s, sa); err != nil {
			continue
		}
		if i == 0 {
			p.ipv6Enabled = true
		} else {
			p.ipv4MappedIPv6Enabled = true
		}
	}
	return
}

func sockaddr(a *net.TCPAddr, family int) (syscall.Sockaddr, error) {
	if a == nil {
		return nil, nil
	}
	return ipToSockaddr(family, a.IP, a.Port, a.Zone)
}

func ipToSockaddr(family int, ip net.IP, port int, zone string) (syscall.Sockaddr, error) {
	switch family {
	case syscall.AF_INET:
		if len(ip) == 0 {
			ip = net.IPv4zero
		}
		ip4 := ip.To4()
		if ip4 == nil {
			return nil, &net.AddrError{Err: "non-IPv4 address", Addr: ip.String()}
		}
		sa := &syscall.SockaddrInet4{Port: port}
		copy(sa.Addr[:], ip4)
		return sa, nil
	case syscall.AF_INET6:
		// In general, an IP wildcard address, which is either
		// "0.0.0.0" or "::", means the entire IP addressing
		// space. For some historical reason, it is used to
		// specify "any available address" on some operations
		// of IP node.
		//
		// When the IP node supports IPv4-mapped IPv6 address,
		// we allow an listener to listen to the wildcard
		// address of both IP addressing spaces by specifying
		// IPv6 wildcard address.
		if len(ip) == 0 || ip.Equal(net.IPv4zero) {
			ip = net.IPv6zero
		}
		// We accept any IPv6 address including IPv4-mapped
		// IPv6 address.
		ip6 := ip.To16()
		if ip6 == nil {
			return nil, &net.AddrError{Err: "non-IPv6 address", Addr: ip.String()}
		}
		sa := &syscall.SockaddrInet6{Port: port, ZoneId: 0}
		copy(sa.Addr[:], ip6)
		return sa, nil
	}
	return nil, &net.AddrError{Err: "invalid address family", Addr: ip.String()}
}
