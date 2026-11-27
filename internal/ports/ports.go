// Package ports checks the local port of a tunnel before the tunnel listens on it, and
// says in plain words, naming the port, why a port cannot be used. A tunnel listens on
// 127.0.0.1 only, so every check here stays on that address and never opens a port to
// the network.
package ports

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"syscall"
	"time"
)

// Host is the only address a tunnel listens on.
const Host = "127.0.0.1"

// lookAhead is how many ports after a busy one Suggest tries.
const lookAhead = 20

// dialTimeout bounds the look for a program that already answers on the port. A program
// that listens on the loopback address answers at once, and a free port is refused at
// once.
const dialTimeout = 250 * time.Millisecond

// Kind says why a port cannot be used.
type Kind int

const (
	// InUse means another program listens on the port.
	InUse Kind = iota + 1
	// Forbidden means the system does not allow listening on the port.
	Forbidden
	// Unknown means the check failed for another reason; Err says which.
	Unknown
)

// Error says why a tunnel cannot listen on a port. Its text is a plain sentence that
// names the port and says what to do, ready for the window.
type Error struct {
	Port int
	Kind Kind
	Err  error
}

func (e *Error) Error() string {
	switch e.Kind {
	case InUse:
		return fmt.Sprintf("Port %d on this computer is already in use by another program. "+
			"Pick another port for this tunnel, or close that program.", e.Port)
	case Forbidden:
		return fmt.Sprintf("This computer does not allow listening on port %d. On Windows that "+
			"usually means the port is in a range the system keeps for itself, for example for "+
			"Hyper-V. Pick another port for this tunnel.", e.Port)
	default:
		return fmt.Sprintf("Port %d on this computer could not be checked: %v", e.Port, e.Err)
	}
}

func (e *Error) Unwrap() error { return e.Err }

// probe holds the two network calls of a check, so tests can stand in for them.
type probe struct {
	dial   func(network, address string, timeout time.Duration) (net.Conn, error)
	listen func(network, address string) (net.Listener, error)
}

var system = probe{dial: net.DialTimeout, listen: net.Listen}

// Reserve makes sure no program answers on 127.0.0.1:port, then listens there and hands
// the listener to the caller, who closes it. An engine that listens through Reserve
// keeps the port from the check on, so nothing can take it in between.
func Reserve(port int) (net.Listener, error) { return system.reserve(port) }

// Check reports whether a tunnel could listen on 127.0.0.1:port now: nil, or an *Error
// that names the port. The port is free again when Check returns.
func Check(port int) error { return system.check(port) }

// Suggest returns the first port after from, up to 20 ports on, that Check finds free
// and avoid does not rule out; 0 when there is none. avoid may be nil.
func Suggest(from int, avoid func(port int) bool) int { return system.suggest(from, avoid) }

func (p probe) reserve(port int) (net.Listener, error) {
	if port < 1 || port > 65535 {
		return nil, &Error{Port: port, Kind: Unknown, Err: errors.New("it is not a TCP port number")}
	}
	address := net.JoinHostPort(Host, strconv.Itoa(port))
	// Look for a program that answers before listening: on Windows a program of the same
	// user that listens on all addresses does not stop a listen on 127.0.0.1, and the
	// tunnel would then take that program's local connections.
	if conn, err := p.dial("tcp", address, dialTimeout); err == nil {
		conn.Close()
		return nil, &Error{Port: port, Kind: InUse}
	}
	listener, err := p.listen("tcp", address)
	if err != nil {
		return nil, classify(port, err)
	}
	return listener, nil
}

func (p probe) check(port int) error {
	listener, err := p.reserve(port)
	if err != nil {
		return err
	}
	return listener.Close()
}

func (p probe) suggest(from int, avoid func(port int) bool) int {
	for port := from + 1; port <= from+lookAhead && port <= 65535; port++ {
		if avoid != nil && avoid(port) {
			continue
		}
		if p.check(port) == nil {
			return port
		}
	}
	return 0
}

// classify sorts a failed listen into a kind.
func classify(port int, err error) *Error {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case errInUse:
			return &Error{Port: port, Kind: InUse, Err: err}
		case errForbidden:
			return &Error{Port: port, Kind: Forbidden, Err: err}
		}
	}
	return &Error{Port: port, Kind: Unknown, Err: err}
}
