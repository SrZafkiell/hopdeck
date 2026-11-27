package tunnel

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

// The states and events of the plan (M04-04), written here again on purpose: the tests
// hold the table to this list instead of trusting the table to describe itself.
var (
	allStates = []State{Idle, Checking, Starting, Connected, Reconnecting, Failed}
	allEvents = []Event{Connect, Checked, Opened, Dropped, Errored, Disconnect}
)

// want is every move the tunnel may make, written independently of the table.
var want = map[State]map[Event]State{
	Idle:         {Connect: Checking},
	Checking:     {Checked: Starting, Errored: Failed, Disconnect: Idle},
	Starting:     {Opened: Connected, Errored: Failed, Disconnect: Idle},
	Connected:    {Dropped: Reconnecting, Errored: Failed, Disconnect: Idle},
	Reconnecting: {Opened: Connected, Dropped: Reconnecting, Errored: Failed, Disconnect: Idle},
	Failed:       {Connect: Checking},
}

// machineAt returns a machine that stands in state from with a reason of its own.
func machineAt(from State) *Machine {
	return &Machine{status: Status{State: from, Reason: "reason before the event"}}
}

// TestEveryPair fires every event in every state: a pair the plan allows moves the
// tunnel to the expected state with the table's reason, and every other pair is refused
// and leaves the tunnel as it was.
func TestEveryPair(t *testing.T) {
	allowed := 0
	for _, from := range allStates {
		for _, event := range allEvents {
			to, ok := want[from][event]
			t.Run(string(from)+"+"+string(event), func(t *testing.T) {
				move, found := Next(from, event)
				if found != ok || (ok && (move.To != to || move.From != from || move.On != event)) {
					t.Fatalf("Next: got %+v, %v; want %s, %v", move, found, to, ok)
				}

				machine := machineAt(from)
				status, err := machine.Fire(event, "")
				if !ok {
					if !errors.Is(err, ErrRefused) {
						t.Fatalf("a refused event gave %v", err)
					}
					if status.State != from || status.Reason != "reason before the event" || machine.Status() != status {
						t.Fatalf("a refused event changed the tunnel: %+v", machine.Status())
					}
					return
				}
				if err != nil || status.State != to || status.Reason != move.Reason || machine.Status() != status {
					t.Fatalf("got %+v, %v; want %s with %q", status, err, to, move.Reason)
				}
			})
			if ok {
				allowed++
			}
		}
	}
	if len(transitions) != allowed {
		t.Errorf("the table has %d rows, the plan allows %d moves: a row is repeated or extra", len(transitions), allowed)
	}
}

// TestTheTableUsesThePlansNames keeps the table to the six states and six events.
func TestTheTableUsesThePlansNames(t *testing.T) {
	states := map[State]bool{}
	for _, state := range allStates {
		states[state] = true
	}
	events := map[Event]bool{}
	for _, event := range allEvents {
		events[event] = true
	}
	for _, row := range Transitions() {
		if !states[row.From] || !states[row.To] || !events[row.On] {
			t.Errorf("a row outside the plan's states and events: %+v", row)
		}
	}
}

// TestEveryStateCarriesAReason checks that no move and no new tunnel leaves the reason
// empty, and that the reasons are plain sentences.
func TestEveryStateCarriesAReason(t *testing.T) {
	reasons := map[State]bool{}
	start := New().Status()
	if start.State != Idle || start.Reason == "" {
		t.Fatalf("a new tunnel: %+v", start)
	}
	reasons[start.State] = true
	for _, row := range Transitions() {
		if row.Reason == "" || !strings.HasSuffix(row.Reason, ".") || strings.ToUpper(row.Reason[:1]) != row.Reason[:1] {
			t.Errorf("not a plain sentence: %+v", row)
		}
		for _, r := range row.Reason {
			if r > 0x7f {
				t.Errorf("not ASCII: %+v", row)
				break
			}
		}
		reasons[row.To] = true
	}
	for _, state := range allStates {
		if !reasons[state] {
			t.Errorf("no reason is ever given for %s", state)
		}
	}
}

// TestNoStateIsATrap checks that every state can be reached from a new tunnel and that a
// tunnel can get back to idle from every state.
func TestNoStateIsATrap(t *testing.T) {
	reach := func(from State, forward bool) map[State]bool {
		seen := map[State]bool{from: true}
		for queue := []State{from}; len(queue) > 0; queue = queue[1:] {
			for _, row := range Transitions() {
				a, b := row.From, row.To
				if !forward {
					a, b = b, a
				}
				if a == queue[0] && !seen[b] {
					seen[b] = true
					queue = append(queue, b)
				}
			}
		}
		return seen
	}
	fromIdle, toIdle := reach(Idle, true), reach(Idle, false)
	for _, state := range allStates {
		if !fromIdle[state] {
			t.Errorf("%s cannot be reached from idle", state)
		}
		if !toIdle[state] {
			t.Errorf("idle cannot be reached from %s", state)
		}
	}
}

func TestAnEventsOwnReasonReplacesTheTables(t *testing.T) {
	machine := New()
	if _, err := machine.Fire(Connect, ""); err != nil {
		t.Fatal(err)
	}
	busy := "Port 15432 on this computer is already in use."
	status, err := machine.Fire(Errored, "  "+busy+"\n")
	if err != nil || status.State != Failed || status.Reason != busy {
		t.Fatalf("got %+v, %v", status, err)
	}
	status, err = machine.Fire(Connect, " \t ")
	if move, _ := Next(Failed, Connect); err != nil || status.Reason != move.Reason {
		t.Fatalf("a blank reason must give the table's: %+v, %v", status, err)
	}
}

// TestALifeOfATunnel walks one tunnel through a working day: connect, two drops (the
// second needs two tries), a failure, a new attempt and a disconnect.
func TestALifeOfATunnel(t *testing.T) {
	machine := New()
	steps := []struct {
		event Event
		want  State
	}{
		{Connect, Checking},
		{Checked, Starting},
		{Opened, Connected},
		{Dropped, Reconnecting},
		{Opened, Connected},
		{Dropped, Reconnecting},
		{Dropped, Reconnecting},
		{Opened, Connected},
		{Dropped, Reconnecting},
		{Errored, Failed},
		{Connect, Checking},
		{Checked, Starting},
		{Opened, Connected},
		{Disconnect, Idle},
	}
	for i, step := range steps {
		status, err := machine.Fire(step.event, "")
		if err != nil || status.State != step.want {
			t.Fatalf("step %d, %s: got %+v, %v; want %s", i+1, step.event, status, err, step.want)
		}
	}
	if _, err := machine.Fire(Disconnect, ""); !errors.Is(err, ErrRefused) {
		t.Fatalf("disconnecting an idle tunnel: %v", err)
	}
}

// TestRacingConnectsMoveTheTunnelOnce fires Connect from many goroutines at once: one
// moves the tunnel to checking and every other one is refused.
func TestRacingConnectsMoveTheTunnelOnce(t *testing.T) {
	machine := New()
	const racers = 64
	var wg sync.WaitGroup
	var mu sync.Mutex
	moved, refused := 0, 0
	for range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := machine.Fire(Connect, "")
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				moved++
			case errors.Is(err, ErrRefused):
				refused++
			}
		}()
	}
	wg.Wait()
	if moved != 1 || refused != racers-1 || machine.Status().State != Checking {
		t.Fatalf("moved %d, refused %d, state %s", moved, refused, machine.Status().State)
	}
}

func TestTransitionsIsACopy(t *testing.T) {
	copied := Transitions()
	copied[0].To = Failed
	if move, _ := Next(Idle, Connect); move.To != Checking {
		t.Fatal("changing the copy changed the table")
	}
}
