package ports

import (
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// freePort returns a port that nothing listened on a moment ago.
func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", Host+":0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return port
}

// hold listens on a loopback port until the test ends and returns the port.
func hold(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", Host+":0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	return listener.Addr().(*net.TCPAddr).Port
}

func kindOf(t *testing.T, err error) Kind {
	t.Helper()
	var portErr *Error
	if !errors.As(err, &portErr) {
		t.Fatalf("not a port error: %v", err)
	}
	return portErr.Kind
}

func TestAFreePortIsFree(t *testing.T) {
	port := freePort(t)
	if err := Check(port); err != nil {
		t.Fatalf("Check(%d): %v", port, err)
	}
	// Check leaves the port free.
	if err := Check(port); err != nil {
		t.Fatalf("second Check(%d): %v", port, err)
	}
}

func TestAPortInUseIsFoundAndNamed(t *testing.T) {
	port := hold(t)
	err := Check(port)
	if kindOf(t, err) != InUse {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(err.Error(), "Port "+strconv.Itoa(port)+" ") {
		t.Fatalf("the message does not name the port: %q", err)
	}

	// The listen alone finds it too, when nothing answers the look before it.
	noAnswer := probe{
		dial: func(string, string, time.Duration) (net.Conn, error) {
			return nil, errors.New("refused")
		},
		listen: net.Listen,
	}
	if err := noAnswer.check(port); kindOf(t, err) != InUse {
		t.Fatalf("listen on a busy port: %v", err)
	}
}

func TestReserveListensOnLoopbackOnly(t *testing.T) {
	port := freePort(t)
	listener, err := Reserve(port)
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().(*net.TCPAddr)
	if !address.IP.Equal(net.ParseIP(Host)) || address.Port != port {
		t.Fatalf("listens on %v", address)
	}
	if err := Check(port); kindOf(t, err) != InUse {
		t.Fatalf("a reserved port must be in use: %v", err)
	}
	listener.Close()
	if err := Check(port); err != nil {
		t.Fatalf("a released port must be free: %v", err)
	}
}

// TestAProgramThatAnswersIsFound stands in for a program that listens on all addresses:
// the system would let Hopdeck listen on 127.0.0.1 next to it, but the look before the
// listen finds it, and the listen is never tried.
func TestAProgramThatAnswersIsFound(t *testing.T) {
	listened := false
	answers := probe{
		dial: func(network, address string, _ time.Duration) (net.Conn, error) {
			if network != "tcp" || address != "127.0.0.1:15432" {
				t.Errorf("dialled %s %s", network, address)
			}
			ours, theirs := net.Pipe()
			theirs.Close()
			return ours, nil
		},
		listen: func(string, string) (net.Listener, error) {
			listened = true
			return nil, errors.New("not expected")
		},
	}
	if err := answers.check(15432); kindOf(t, err) != InUse || listened {
		t.Fatalf("got %v, listened %v", err, listened)
	}
}

func TestClassify(t *testing.T) {
	failedListen := func(err error) error {
		return &net.OpError{Op: "listen", Net: "tcp", Err: os.NewSyscallError("bind", err)}
	}
	cases := []struct {
		name string
		err  error
		kind Kind
		says string
	}{
		{"busy", failedListen(errInUse), InUse, "already in use by another program"},
		{"refused by the system", failedListen(errForbidden), Forbidden, "does not allow listening on port 15432"},
		{"another errno", failedListen(syscall.Errno(1)), Unknown, "could not be checked"},
		{"no errno", errors.New("something else"), Unknown, "could not be checked: something else"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sorted := classify(15432, tc.err)
			if sorted.Kind != tc.kind || sorted.Port != 15432 || !errors.Is(sorted, tc.err) {
				t.Fatalf("got %+v", sorted)
			}
			text := sorted.Error()
			if !strings.Contains(text, tc.says) || !strings.Contains(text, "15432") {
				t.Fatalf("message: %q", text)
			}
		})
	}
}

func TestNotAPortNumber(t *testing.T) {
	for _, port := range []int{-1, 0, 65536} {
		if err := Check(port); kindOf(t, err) != Unknown || !strings.Contains(err.Error(), "not a TCP port number") {
			t.Errorf("Check(%d): %v", port, err)
		}
	}
}

func TestSuggest(t *testing.T) {
	// A made-up computer where 15433 and 15434 answer, 15435 is ruled out by the caller
	// and the rest is free.
	busy := map[string]bool{"127.0.0.1:15433": true, "127.0.0.1:15434": true}
	var tried []string
	fake := probe{
		dial: func(_, address string, _ time.Duration) (net.Conn, error) {
			tried = append(tried, address)
			if busy[address] {
				ours, theirs := net.Pipe()
				theirs.Close()
				return ours, nil
			}
			return nil, errors.New("refused")
		},
		listen: func(string, string) (net.Listener, error) { return closer{}, nil },
	}
	avoid := func(port int) bool { return port == 15435 }
	if got := fake.suggest(15432, avoid); got != 15436 {
		t.Fatalf("got %d, tried %v", got, tried)
	}
	for _, address := range tried {
		if address == "127.0.0.1:15435" {
			t.Fatal("a port the caller ruled out was probed")
		}
	}

	everythingBusy := probe{
		dial: func(string, string, time.Duration) (net.Conn, error) {
			ours, theirs := net.Pipe()
			theirs.Close()
			return ours, nil
		},
		listen: fake.listen,
	}
	if got := everythingBusy.suggest(15432, nil); got != 0 {
		t.Fatalf("nothing is free, got %d", got)
	}
	if got := fake.suggest(65535, nil); got != 0 {
		t.Fatalf("no port follows 65535, got %d", got)
	}

	// On the real computer, a suggestion is a free port after the busy one.
	port := hold(t)
	if next := Suggest(port, nil); next <= port || next > port+lookAhead || Check(next) != nil {
		t.Fatalf("Suggest(%d) = %d", port, next)
	}
}

// TestAPortTheSystemRefuses runs only where HOPDECK_TEST_EXCLUDED_PORT names a port the
// system refuses, such as one in a Windows excluded port range (see `netsh interface ipv4
// show excludedportrange protocol=tcp`); the ranges differ from computer to computer.
func TestAPortTheSystemRefuses(t *testing.T) {
	value := os.Getenv("HOPDECK_TEST_EXCLUDED_PORT")
	if value == "" {
		t.Skip("set HOPDECK_TEST_EXCLUDED_PORT to a port the system refuses")
	}
	port, err := strconv.Atoi(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := Check(port); kindOf(t, err) != Forbidden {
		t.Fatalf("Check(%d): %v", port, err)
	}
}

// closer is a listener that only closes.
type closer struct{ net.Listener }

func (closer) Close() error { return nil }
