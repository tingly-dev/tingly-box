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

	folder := &managedagent.Folder{ID: "f-1", Path: "/home/me/code/app", Name: "app", CreatedAt: now, LastUsedAt: now}
	if err := store.CreateFolder(ctx, folder); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetFolder(ctx, "f-1")
	if err != nil || got.Path != folder.Path || got.Name != "app" {
		t.Fatalf("folder round trip: %+v %v", got, err)
	}
	byPath, err := store.GetFolderByPath(ctx, "/home/me/code/app")
	if err != nil || byPath.ID != "f-1" {
		t.Fatalf("lookup by path: %+v %v", byPath, err)
	}
	if _, err := store.GetFolderByPath(ctx, "/nope"); !errors.Is(err, managedagent.ErrNotFound) {
		t.Fatalf("missing path: want ErrNotFound, got %v", err)
	}

	// A second folder used more recently sorts first.
	later := &managedagent.Folder{ID: "f-2", Path: "/home/me/notes", Name: "notes", CreatedAt: now, LastUsedAt: now.Add(time.Minute)}
	if err := store.CreateFolder(ctx, later); err != nil {
		t.Fatal(err)
	}
	list, err := store.ListFolders(ctx)
	if err != nil || len(list) != 2 || list[0].ID != "f-2" {
		t.Fatalf("folders must list most recently used first: %+v %v", list, err)
	}

	sess := &managedagent.Session{
		ID: "s-1", Title: "add tests", FolderID: "f-1", Status: managedagent.SessionRunning,
		Prompt: "add tests", CCSessionID: "cc-1", PermissionMode: managedagent.PermissionAcceptEdits,
		CreatedBy: "web", Usage: managedagent.Usage{InputTokens: 10, OutputTokens: 4, Cost: 0.5},
		BaseCommit: "abc123", ChangedFiles: 2, CreatedAt: now, LastActiveAt: now,
	}
	if err := store.CreateSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	gotSess, err := store.GetSession(ctx, "s-1")
	if err != nil {
		t.Fatal(err)
	}
	if gotSess.FolderID != "f-1" || gotSess.Usage.InputTokens != 10 || gotSess.BaseCommit != "abc123" || gotSess.ChangedFiles != 2 {
		t.Fatalf("session round trip lost fields: %+v", gotSess)
	}

	// An archived session in the other folder: filters must separate them.
	archived := &managedagent.Session{ID: "s-2", FolderID: "f-2", Status: managedagent.SessionArchived, CreatedAt: now, LastActiveAt: now.Add(-time.Hour)}
	if err := store.CreateSession(ctx, archived); err != nil {
		t.Fatal(err)
	}
	active, err := store.ListSessions(ctx, managedagent.SessionFilter{Active: true})
	if err != nil || len(active) != 1 || active[0].ID != "s-1" {
		t.Fatalf("active filter: %+v %v", active, err)
	}
	byFolder, err := store.ListSessions(ctx, managedagent.SessionFilter{FolderID: "f-2"})
	if err != nil || len(byFolder) != 1 || byFolder[0].ID != "s-2" {
		t.Fatalf("folder filter: %+v %v", byFolder, err)
	}
	all, _ := store.ListSessions(ctx, managedagent.SessionFilter{})
	if len(all) != 2 || all[0].ID != "s-1" {
		t.Fatalf("sessions must list most recently active first: %+v", all)
	}

	gotSess.Status = managedagent.SessionIdle
	if err := store.UpdateSession(ctx, gotSess); err != nil {
		t.Fatal(err)
	}
	if again, _ := store.GetSession(ctx, "s-1"); again.Status != managedagent.SessionIdle {
		t.Fatalf("update not persisted: %+v", again)
	}
	if err := store.UpdateSession(ctx, &managedagent.Session{ID: "missing"}); !errors.Is(err, managedagent.ErrNotFound) {
		t.Fatalf("update missing: want ErrNotFound, got %v", err)
	}

	if err := store.DeleteFolder(ctx, "f-2"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetFolder(ctx, "f-2"); !errors.Is(err, managedagent.ErrNotFound) {
		t.Fatalf("delete: want ErrNotFound, got %v", err)
	}
	if err := store.DeleteFolder(ctx, "f-2"); !errors.Is(err, managedagent.ErrNotFound) {
		t.Fatalf("delete missing: want ErrNotFound, got %v", err)
	}
}
