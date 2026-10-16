package db

import "testing"

func newTestGitHubStore(t *testing.T) *GitHubStore {
	t.Helper()
	sm, err := NewStoreManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewStoreManager: %v", err)
	}
	t.Cleanup(func() { _ = sm.Close() })
	s := sm.GitHub()
	if s == nil {
		t.Fatal("GitHub store not initialized")
	}
	return s
}

func TestGitHubStore_AppReplaceAndDelete(t *testing.T) {
	s := newTestGitHubStore(t)

	if app, err := s.App(); err != nil || app != nil {
		t.Fatalf("empty store: app=%v err=%v", app, err)
	}
	if err := s.SaveApp(GitHubAppRecord{AppID: 1, Slug: "one", PrivateKey: "pem1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveInstallation(GitHubInstallationRecord{InstallationID: 10, AppID: 1, Account: "me"}); err != nil {
		t.Fatal(err)
	}

	// A new App replaces the old one and its installations.
	if err := s.SaveApp(GitHubAppRecord{AppID: 2, Slug: "two", PrivateKey: "pem2"}); err != nil {
		t.Fatal(err)
	}
	app, _ := s.App()
	if app == nil || app.AppID != 2 || app.PrivateKey != "pem2" {
		t.Fatalf("app = %+v", app)
	}
	if ins, _ := s.Installations(); len(ins) != 0 {
		t.Fatalf("installations of the old app survived: %+v", ins)
	}

	if err := s.DeleteApp(); err != nil {
		t.Fatal(err)
	}
	if app, _ := s.App(); app != nil {
		t.Fatalf("app after delete = %+v", app)
	}
}

func TestGitHubStore_InstallationsUpsertAndReplace(t *testing.T) {
	s := newTestGitHubStore(t)
	_ = s.SaveInstallation(GitHubInstallationRecord{InstallationID: 10, Account: "a", RepositorySelection: "all"})
	_ = s.SaveInstallation(GitHubInstallationRecord{InstallationID: 10, Account: "a", RepositorySelection: "selected"})
	ins, _ := s.Installations()
	if len(ins) != 1 || ins[0].RepositorySelection != "selected" {
		t.Fatalf("upsert: %+v", ins)
	}
	if err := s.ReplaceInstallations([]GitHubInstallationRecord{{InstallationID: 11, Account: "b"}}); err != nil {
		t.Fatal(err)
	}
	ins, _ = s.Installations()
	if len(ins) != 1 || ins[0].InstallationID != 11 {
		t.Fatalf("replace: %+v", ins)
	}
}

func TestGitHubStore_PATAndSessionPR(t *testing.T) {
	s := newTestGitHubStore(t)
	if tok, _ := s.PAT(); tok != "" {
		t.Fatalf("pat = %q", tok)
	}
	_ = s.SetPAT("ghp_x")
	_ = s.SetPAT("ghp_y")
	if tok, _ := s.PAT(); tok != "ghp_y" {
		t.Fatalf("pat = %q", tok)
	}
	_ = s.SetPAT("")
	if tok, _ := s.PAT(); tok != "" {
		t.Fatalf("pat after clear = %q", tok)
	}

	if pr, _ := s.SessionPR("s1", "o/r"); pr != nil {
		t.Fatalf("unexpected pr %+v", pr)
	}
	_ = s.SaveSessionPR(GitHubSessionPRRecord{SessionID: "s1", Repo: "o/r", Branch: "tb/s1"})
	_ = s.SaveSessionPR(GitHubSessionPRRecord{SessionID: "s1", Repo: "o/r", Branch: "tb/s1", PRNumber: 7})
	pr, _ := s.SessionPR("s1", "o/r")
	if pr == nil || pr.PRNumber != 7 {
		t.Fatalf("pr = %+v", pr)
	}
}
