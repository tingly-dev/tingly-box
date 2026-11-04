package project

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestCoordinatorViewStatus(t *testing.T) {
	main := &db.Thread{ID: "m", Status: "stale", Turns: 3}
	require.Equal(t, db.ThreadWorking, coordinatorView(main, true, "").Status)
	require.Equal(t, db.ThreadIdle, coordinatorView(main, false, "").Status)
	require.Equal(t, db.ThreadWaiting, coordinatorView(main, true, "input-1").Status, "a decision outranks a running turn")
	require.Equal(t, "stale", main.Status, "the stored row is left alone")
	require.Equal(t, 3, coordinatorView(main, false, "").Turns)
	require.Nil(t, coordinatorView(nil, true, ""))
}
