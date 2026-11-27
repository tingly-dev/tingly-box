package project

import (
	"runtime"
	"testing"
)

func TestSandboxRuntimesMatchPlatform(t *testing.T) {
	for name, platform := range map[string]string{
		"bwrap": "linux", "microvm": "linux",
		"macvm": "darwin", "seatbelt": "darwin",
	} {
		if got, want := ValidSandbox(name), runtime.GOOS == platform; got != want {
			t.Errorf("ValidSandbox(%q) = %v on %s, want %v", name, got, runtime.GOOS, want)
		}
	}
}
