package daemon

import (
	"fmt"
	"sync"

	"github.com/rs/zerolog"
)

// State represents a lifecycle state of the daemon FSM.
type State string

const (
	StateInit      State = "INIT"
	StatePreflight State = "PREFLIGHT"
	StateIdle      State = "IDLE"
	StateExecuting State = "EXECUTING"
	StatePaused    State = "PAUSED"
	StateShutdown  State = "SHUTDOWN"
)

// allowedTransitions defines valid state transitions.
var allowedTransitions = map[State][]State{
	StateInit:      {StatePreflight},
	StatePreflight: {StateIdle, StateShutdown},
	StateIdle:      {StateExecuting, StatePaused, StateShutdown},
	StateExecuting: {StateIdle, StateShutdown},
	StatePaused:    {StateIdle, StateShutdown},
	StateShutdown:  {}, // terminal state
}

// FSM is the finite state machine managing the daemon lifecycle.
type FSM struct {
	state State
	mu    sync.RWMutex
	log   zerolog.Logger
}

// NewFSM creates a new FSM in the INIT state.
func NewFSM(log zerolog.Logger) *FSM {
	return &FSM{
		state: StateInit,
		log:   log.With().Str("component", "lifecycle-fsm").Logger(),
	}
}

// Transition attempts to move the FSM from `from` to `to`.
// Returns an error if the current state does not match `from` or the transition is not allowed.
func (f *FSM) Transition(from, to State) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.state != from {
		return fmt.Errorf("FSM transition failed: expected current state %s, got %s", from, f.state)
	}

	// SHUTDOWN is allowed from any state
	if to == StateShutdown {
		f.log.Info().
			Str("from", string(f.state)).
			Str("to", string(to)).
			Msg("FSM state transition (shutdown override)")
		f.state = to
		return nil
	}

	allowed := allowedTransitions[from]
	for _, s := range allowed {
		if s == to {
			f.log.Info().
				Str("from", string(from)).
				Str("to", string(to)).
				Msg("FSM state transition")
			f.state = to
			return nil
		}
	}

	return fmt.Errorf("FSM transition %s → %s is not allowed", from, to)
}

// State returns the current FSM state.
func (f *FSM) State() State {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.state
}

// String returns the string representation of the current state.
func (f *FSM) String() string {
	return string(f.State())
}
