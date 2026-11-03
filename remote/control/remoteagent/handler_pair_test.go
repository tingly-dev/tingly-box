package remoteagent

import "testing"

// TestPairingHelpers_isPairCommand exercises the parser used by the gate.
func TestPairingHelpers_isPairCommand(t *testing.T) {
	cases := map[string]bool{
		"/pair":           true,
		"/pair ":          true,
		"/pair ABCD-EFGH": true,
		"  /pair ABCD":    true, // helper trims internally
		"/pairing":        false,
		"hello":           false,
		"/cd /tmp":        false,
		"/pair\tABCD":     true,
		"/bind ABCD":      true, // compatibility for existing pairing instructions
		"/binding":        false,
	}
	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			if got := isPairCommand(input); got != want {
				t.Fatalf("isPairCommand(%q)=%v want %v", input, got, want)
			}
		})
	}
}
