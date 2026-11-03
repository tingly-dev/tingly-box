package settings

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestCoordinatorEnvRejectsOpenAIConnection(t *testing.T) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	svc := NewService(store)
	require.NoError(t, svc.Save(Connection{APIStyle: "openai", BaseURL: "http://localhost:1234"}))
	_, _, err = svc.CoordinatorEnv(context.Background())
	require.ErrorContains(t, err, "Anthropic-compatible")
}
