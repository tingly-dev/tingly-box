package commands

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAliasesDoNotCollide(t *testing.T) {
	canonical := map[string]bool{}
	for _, name := range []string{"help", "pair", "join", "cd", "dir", "project", "session", "new", "stop", "clear", "status", "verbose", "yolo"} {
		canonical[name] = true
	}
	seen := map[string]bool{}
	for _, alias := range aliases {
		require.Regexp(t, `^[a-z][a-z0-9_]*$`, alias.Name)
		require.False(t, canonical[alias.Name], "alias shadows command: %s", alias.Name)
		require.False(t, seen[alias.Name], "duplicate alias: %s", alias.Name)
		require.True(t, canonical[alias.Command], "unknown target: %s", alias.Command)
		seen[alias.Name] = true
		require.Equal(t, alias.Command, Canonical(alias.Name))
	}
	for name := range canonical {
		require.Equal(t, name, Canonical(name))
	}
	require.Equal(t, "project", Canonical("p"))
	require.Equal(t, "clear", Canonical("cls"))
	require.Empty(t, Aliases("stop"))
	require.Equal(t, "session", Canonical("s"))
	for _, name := range []string{"sess", "sp", "cr", "unknown"} {
		require.Equal(t, name, Canonical(name))
	}
}
