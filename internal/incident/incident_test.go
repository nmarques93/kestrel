package incident

import "testing"

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name      string
		current   State
		recent    []bool // newest first
		threshold int
		want      Transition
	}{
		{
			name:      "not enough history yet",
			current:   Up,
			recent:    []bool{false, false},
			threshold: 3,
			want:      NoTransition,
		},
		{
			name:      "exactly threshold consecutive failures trips to down",
			current:   Up,
			recent:    []bool{false, false, false},
			threshold: 3,
			want:      ToDown,
		},
		{
			name:      "one short of threshold does not trip",
			current:   Up,
			recent:    []bool{false, false, true},
			threshold: 3,
			want:      NoTransition,
		},
		{
			name:      "alternating pass/fail never trips despite long history",
			current:   Up,
			recent:    []bool{false, true, false, true, false, true, false},
			threshold: 3,
			want:      NoTransition,
		},
		{
			name:      "already down stays down without enough consecutive successes",
			current:   Down,
			recent:    []bool{true, true, false},
			threshold: 3,
			want:      NoTransition,
		},
		{
			name:      "exactly threshold consecutive successes recovers to up",
			current:   Down,
			recent:    []bool{true, true, true},
			threshold: 3,
			want:      ToUp,
		},
		{
			name:      "more than threshold consecutive failures still trips (only trailing window matters)",
			current:   Up,
			recent:    []bool{false, false, false, false, false},
			threshold: 3,
			want:      ToDown,
		},
		{
			name:      "already up with all successes is a no-op, not a transition",
			current:   Up,
			recent:    []bool{true, true, true},
			threshold: 3,
			want:      NoTransition,
		},
		{
			name:      "already down with all failures is a no-op, not a transition",
			current:   Down,
			recent:    []bool{false, false, false},
			threshold: 3,
			want:      NoTransition,
		},
		{
			name:      "threshold of 1 trips on a single result",
			current:   Up,
			recent:    []bool{false},
			threshold: 1,
			want:      ToDown,
		},
		{
			name:      "zero threshold never trips",
			current:   Up,
			recent:    []bool{false, false, false},
			threshold: 0,
			want:      NoTransition,
		},
		{
			name:      "no history never trips",
			current:   Up,
			recent:    nil,
			threshold: 3,
			want:      NoTransition,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Evaluate(tt.current, tt.recent, tt.threshold)
			if got != tt.want {
				t.Errorf("Evaluate(%v, %v, %d) = %v, want %v", tt.current, tt.recent, tt.threshold, got, tt.want)
			}
		})
	}
}

// TestEvaluateSequence drives Evaluate the way the writer goroutine actually
// will: once per new check, feeding it the trailing window of results ending
// at that check. This is what proves flapping doesn't cause rapid
// transitions in practice, not just in a single call.
func TestEvaluateSequence(t *testing.T) {
	threshold := 3
	checks := []bool{true, true, true, false, true, false, true, false, false, false, true, true, true}
	// Expected running state after each check, derived by hand from the
	// rule "flip after `threshold` consecutive identical results":
	// idx0 T -> Up (starts Up)
	// idx1 T -> Up
	// idx2 T -> Up
	// idx3 F -> Up (only 1 failure)
	// idx4 T -> Up
	// idx5 F -> Up (1 failure)
	// idx6 T -> Up
	// idx7 F -> Up (1 failure)
	// idx8 F -> Up (2 failures)
	// idx9 F -> Down (3 consecutive failures)
	// idx10 T -> Down (1 success)
	// idx11 T -> Down (2 successes)
	// idx12 T -> Up (3 consecutive successes)
	wantStates := []State{Up, Up, Up, Up, Up, Up, Up, Up, Up, Down, Down, Down, Up}

	state := Up
	for i := range checks {
		recent := reverse(checks[:i+1])
		transition := Evaluate(state, recent, threshold)
		switch transition {
		case ToDown:
			state = Down
		case ToUp:
			state = Up
		}
		if state != wantStates[i] {
			t.Fatalf("after check %d (result=%v): state = %v, want %v", i, checks[i], state, wantStates[i])
		}
	}
}

// reverse returns a copy of checks in newest-first order, trimmed to the
// window Evaluate actually needs (it only ever looks at the first few
// elements, but the trimming mirrors how the store package will query
// "last N checks" from Postgres).
func reverse(checks []bool) []bool {
	out := make([]bool, len(checks))
	for i, v := range checks {
		out[len(checks)-1-i] = v
	}
	return out
}
