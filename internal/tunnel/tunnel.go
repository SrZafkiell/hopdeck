// Package tunnel holds the life of one tunnel as a small state machine. The states, the
// events that move a tunnel between them and the reason that comes with every move are
// one table, so neither the engine nor the window decides a state on its own: the
// engine reports what happened, the person asks for something, and the table says where
// the tunnel goes and why.
package tunnel

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// State is where a tunnel is.
type State string

const (
	// Idle means nothing runs for the tunnel.
	Idle State = "idle"
	// Checking means the local port and the credentials are checked before a session
	// starts.
	Checking State = "checking"
	// Starting means a Session Manager session is being opened.
	Starting State = "starting"
	// Connected means the local port forwards through the jump host.
	Connected State = "connected"
	// Reconnecting means the session ended without a Disconnect and a new one is being
	// opened on the same local port.
	Reconnecting State = "reconnecting"
	// Failed means the tunnel stopped because of a problem that a new attempt will not
	// fix by itself; the reason says what the problem was.
	Failed State = "failed"
)

// Event is what happens to a tunnel: a request of the person, or a report of the engine.
type Event string

const (
	// Connect is the person asking for the tunnel.
	Connect Event = "connect"
	// Checked reports that the checks before a session passed.
	Checked Event = "checked"
	// Opened reports that a session is open and the local port forwards.
	Opened Event = "opened"
	// Dropped reports that the session ended without a Disconnect, for example after
	// the idle timeout, the maximum session duration or a network change.
	Dropped Event = "dropped"
	// Errored reports a problem that a new attempt will not fix by itself, such as a
	// busy local port, expired credentials or a refused permission.
	Errored Event = "errored"
	// Disconnect is the person, or the app as it closes, asking for the tunnel to stop.
	Disconnect Event = "disconnect"
)

// Transition is one move of the table.
type Transition struct {
	From State
	On   Event
	To   State
	// Reason is the tunnel's reason after the move, unless the event brings its own.
	Reason string
}

// The reasons more than one move shares.
const (
	reasonIdle     = "Nothing runs for this tunnel yet."
	reasonChecking = "Checking the local port and the AWS credentials."
	reasonStopped  = "Stopped before the session opened."
	reasonEnded    = "Disconnected. Connect to open it again."
)

// transitions is every move a tunnel can make. A state and an event that are not paired
// here are refused, and the tunnel stays as it was. A failed tunnel has one way out,
// Connect, so a failure stays on screen until the person acts on it.
var transitions = []Transition{
	{Idle, Connect, Checking, reasonChecking},

	{Checking, Checked, Starting, "Opening a Session Manager session through the jump host."},
	{Checking, Errored, Failed, "A check before the session failed."},
	{Checking, Disconnect, Idle, reasonStopped},

	{Starting, Opened, Connected, "Traffic to the local address goes through the jump host."},
	{Starting, Errored, Failed, "The session could not be opened."},
	{Starting, Disconnect, Idle, reasonStopped},

	{Connected, Dropped, Reconnecting, "The session ended. Opening a new one on the same local port."},
	{Connected, Errored, Failed, "The tunnel stopped because of an error."},
	{Connected, Disconnect, Idle, reasonEnded},

	{Reconnecting, Opened, Connected, "Reconnected. Traffic to the local address goes through the jump host again."},
	{Reconnecting, Dropped, Reconnecting, "The new session has not opened yet. Trying again."},
	{Reconnecting, Errored, Failed, "The tunnel could not reconnect."},
	{Reconnecting, Disconnect, Idle, reasonEnded},

	{Failed, Connect, Checking, reasonChecking},
}

// ErrRefused is returned for an event that the table does not pair with the tunnel's
// state.
var ErrRefused = errors.New("the event does not apply to the tunnel's state")

// Next looks a move up in the table: where a tunnel in state from goes on event, and
// why. ok is false when the table does not pair them.
func Next(from State, event Event) (move Transition, ok bool) {
	for _, row := range transitions {
		if row.From == from && row.On == event {
			return row, true
		}
	}
	return Transition{}, false
}

// Transitions returns a copy of the table.
func Transitions() []Transition {
	return append([]Transition(nil), transitions...)
}

// Status is where a tunnel is and why, in plain words.
type Status struct {
	State  State
	Reason string
}

// Machine is the state of one tunnel. Use New to get one; it is safe for concurrent use,
// and every event is applied to the state as it is at that moment, so of two events
// that race only the one the table allows first moves the tunnel.
type Machine struct {
	mu     sync.Mutex
	status Status
}

// New returns a machine for a tunnel that is idle.
func New() *Machine {
	return &Machine{status: Status{State: Idle, Reason: reasonIdle}}
}

// Status returns where the tunnel is now.
func (m *Machine) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

// Fire moves the tunnel by the table and returns its new status. reason, when not
// blank, replaces the table's reason, so the engine can say what exactly happened. An
// event that the table does not pair with the current state is refused with ErrRefused,
// and the status stays as it was.
func (m *Machine) Fire(event Event, reason string) (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	move, ok := Next(m.status.State, event)
	if !ok {
		return m.status, fmt.Errorf("%w: %s while %s", ErrRefused, event, m.status.State)
	}
	if reason = strings.TrimSpace(reason); reason == "" {
		reason = move.Reason
	}
	m.status = Status{State: move.To, Reason: reason}
	return m.status, nil
}
