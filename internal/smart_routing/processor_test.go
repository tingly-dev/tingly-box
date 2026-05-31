package smartrouting

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Harness — fakeOpProcessor
//
// The op-processor registry is a generic mechanism; these tests exercise it
// with synthetic position/op values rather than any real op (the vision
// proxy op that used to live here was removed in favor of the rule/scenario
// vision proxy flags).
// ---------------------------------------------------------------------------

const (
	testPosition  SmartOpPosition  = "_test_position"
	testOpEnabled SmartOpOperation = "_test_enabled"
)

type fakeOpProcessor struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeOpProcessor) Process(_ *ProcessorContext) error {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	return nil
}

func registerFakeProcessor(t *testing.T, pos SmartOpPosition, op SmartOpOperation, fake *fakeOpProcessor) {
	t.Helper()
	RegisterProcessor(pos, op, fake)
	t.Cleanup(func() { UnregisterProcessor(pos, op) })
}

// ---------------------------------------------------------------------------
// Tests — registry
// ---------------------------------------------------------------------------

func TestRegisterProcessor_StoresAndLooksUp(t *testing.T) {
	fake := &fakeOpProcessor{}
	registerFakeProcessor(t, testPosition, testOpEnabled, fake)

	got, ok := LookupProcessor(testPosition, testOpEnabled)
	require.True(t, ok)
	require.Same(t, OpProcessor(fake), got)
}

func TestLookupProcessor_MissingReturnsFalse(t *testing.T) {
	got, ok := LookupProcessor(testPosition, "nonexistent_op_xyz")
	require.False(t, ok)
	require.Nil(t, got)
}

func TestRegisterProcessor_OverwriteContract(t *testing.T) {
	// Production contract: silently replace. Keeps server boot idempotent
	// across config reloads.
	first := &fakeOpProcessor{}
	second := &fakeOpProcessor{}

	RegisterProcessor(testPosition, testOpEnabled, first)
	t.Cleanup(func() { UnregisterProcessor(testPosition, testOpEnabled) })

	RegisterProcessor(testPosition, testOpEnabled, second)

	got, ok := LookupProcessor(testPosition, testOpEnabled)
	require.True(t, ok)
	require.Same(t, OpProcessor(second), got, "second registration must replace the first")
}
