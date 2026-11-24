package main

import (
	"strings"
	"testing"
)

// The --help example list is derived from testdata/lb, so a new example file
// shows up without editing the command's help text.
func TestLbHelpListsEmbeddedExamples(t *testing.T) {
	if len(lbExampleNames) == 0 {
		t.Fatal("no embedded lb examples found")
	}
	help := (&LbCmd{}).Help()
	for _, name := range lbExampleNames {
		if !strings.Contains(help, name) {
			t.Errorf("lb help does not list example %q", name)
		}
	}
}
