// Package incident implements the flap-prevention state machine: a target
// only transitions DOWN after N consecutive failed checks, and back UP only
// after N consecutive successful checks. It is a pure function of the
// target's current state and its most recent check results, with no
// knowledge of the database or the HTTP layer.
package incident

// State is a target's current monitored state.
type State int

const (
	Up State = iota
	Down
)

// Transition describes a state change produced by Evaluate.
type Transition int

const (
	NoTransition Transition = iota
	ToDown
	ToUp
)

// Evaluate decides whether a target should transition given its current
// state and its most recent check results, newest first. recent may be
// shorter than threshold, in which case there isn't enough history yet to
// justify a transition.
//
// A single differing result anywhere in the last `threshold` checks resets
// the streak, which is what prevents an alternating pass/fail pattern from
// flipping the state on every check.
func Evaluate(current State, recent []bool, threshold int) Transition {
	if threshold <= 0 || len(recent) < threshold {
		return NoTransition
	}

	window := recent[:threshold]

	switch current {
	case Up:
		if allFailures(window) {
			return ToDown
		}
	case Down:
		if allSuccesses(window) {
			return ToUp
		}
	}

	return NoTransition
}

func allFailures(results []bool) bool {
	for _, r := range results {
		if r {
			return false
		}
	}
	return true
}

func allSuccesses(results []bool) bool {
	for _, r := range results {
		if !r {
			return false
		}
	}
	return true
}
