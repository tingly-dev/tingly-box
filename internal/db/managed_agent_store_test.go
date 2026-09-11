package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/managedagent"
)

func TestManagedAgentStore_RoundTrip(t *testing.T) {
	sm, err := NewStoreManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer sm.Close()
	store := sm.ManagedAgent()
	if store == nil {
		t.Fatal("ManagedAgent store not initialised by StoreManager")
	}
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	// Environment with JSON columns and docker fields survives a round trip.
	env := &managedagent.Environment{
		ID: "env-1", Name: "Box", Runtime: managedagent.RuntimeDocker, Image: "ghcr.io/x/cc:1",
		Env: map[string]string{"FOO": "bar"}, SecretRefs: []string{"gh-token"},
		Network: managedagent.NetworkProxy, Resources: managedagent.Resources{CPU: 2, MemoryMB: 4096},
		CCProfile: "claude_code:p1", CreatedAt: now, UpdatedAt: now,
	}
	if err := store.CreateEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetEnvironment(ctx, "env-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Env["FOO"] != "bar" || len(got.SecretRefs) != 1 || got.Resources.MemoryMB != 4096 || got.Network != managedagent.NetworkProxy {
		t.Fatalf("environment round trip lost fields: %+v", got)
	}
	got.Name = "Box 2"
	if err := store.UpdateEnvironment(ctx, got); err != nil {
		t.Fatal(err)
	}
	if again, _ := store.GetEnvironment(ctx, "env-1"); again.Name != "Box 2" {
		t.Fatalf("update not persisted: %+v", again)
	}
	if err := store.UpdateEnvironment(ctx, &managedagent.Environment{ID: "missing"}); !errors.Is(err, managedagent.ErrNotFound) {
		t.Fatalf("update missing: want ErrNotFound, got %v", err)
	}

	src := &managedagent.Source{ID: "src-1", Name: "repo", Kind: managedagent.SourceKindGit, URL: "https://x/y.git", DefaultBranch: "main", CreatedAt: now, UpdatedAt: now}
	if err := store.CreateSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	ws := &managedagent.Workspace{ID: "ws-1", SourceID: "src-1", EnvironmentID: "env-1", Path: "/tmp/ws", BaseRef: "main", Branch: "tb/x", State: managedagent.WorkspaceReady, CreatedAt: now, LastActiveAt: now}
	if err := store.CreateWorkspace(ctx, ws); err != nil {
		t.Fatal(err)
	}
	list, _ := store.ListWorkspaces(ctx, managedagent.WorkspaceFilter{SourceID: "src-1", State: managedagent.WorkspaceReady})
	if len(list) != 1 {
		t.Fatalf("ListWorkspaces filter: got %d", len(list))
	}

	// Sessions: ordering by activity, Active filter, and artifact/usage columns.
	fin := now
	for i, st := range []managedagent.SessionStatus{managedagent.SessionArchived, managedagent.SessionIdle, managedagent.SessionRunning} {
		s := &managedagent.Session{
			ID: "s-" + string(rune('a'+i)), WorkspaceID: "ws-1", Status: st, Prompt: "p", CreatedBy: "web",
			Usage:     managedagent.Usage{InputTokens: 10, Cost: 0.5},
			Artifact:  managedagent.Artifact{Branch: "tb/x", Pushed: true, PRURL: "https://pr", Changed: 3},
			CreatedAt: now, LastActiveAt: now.Add(time.Duration(i) * time.Minute),
		}
		if st == managedagent.SessionArchived {
			s.FinishedAt = &fin
		}
		if err := store.CreateSession(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	active, err := store.ListSessions(ctx, managedagent.SessionFilter{Active: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 2 || active[0].ID != "s-c" || active[1].ID != "s-b" {
		t.Fatalf("active sessions order: %+v", active)
	}
	one, _ := store.GetSession(ctx, "s-a")
	if one.FinishedAt == nil || !one.Artifact.Pushed || one.Artifact.Changed != 3 || one.Usage.Cost != 0.5 {
		t.Fatalf("session round trip lost fields: %+v", one)
	}
	if _, err := store.GetSession(ctx, "nope"); !errors.Is(err, managedagent.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := store.DeleteSource(ctx, "src-1"); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteSource(ctx, "src-1"); !errors.Is(err, managedagent.ErrNotFound) {
		t.Fatalf("second delete: want ErrNotFound, got %v", err)
	}
}
