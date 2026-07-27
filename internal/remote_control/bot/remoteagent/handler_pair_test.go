package remoteagent

import "testing"

// TestPairingHelpers_isBindCommand exercises the parser used by the gate.
func TestPairingHelpers_isBindCommand(t *testing.T) {
	cases := map[string]bool{
		"/bind":           true,
		"/bind ":          true,
		"/bind ABCD-EFGH": true,
		"  /bind ABCD":    true, // helper trims internally
		"/binding":        false,
		"hello":           false,
		"/cd /tmp":        false,
		"/bind\tABCD":     true,
	}
	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			if got := isBindCommand(input); got != want {
				t.Fatalf("isBindCommand(%q)=%v want %v", input, got, want)
			}
		})
	}
}
