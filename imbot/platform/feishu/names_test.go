package feishu

import (
	"context"
	"errors"
	"github.com/tingly-dev/tingly-box/imbot/core"
	"sync"
	"testing"
	"time"
)

type fakeFetcher struct {
	mu      sync.Mutex
	users   map[string]string
	chats   map[string]string
	err     error
	calls   int
	lastTyp string
}

func (f *fakeFetcher) userName(_ context.Context, idType, id string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastTyp = idType
	return f.users[id], f.err
}
func (f *fakeFetcher) chatName(_ context.Context, id string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.chats[id], f.err
}

func TestNamesAreFetchedOnceAndCached(t *testing.T) {
	f := &fakeFetcher{users: map[string]string{"ou_1": " Alice "}, chats: map[string]string{"oc_1": "Dev team"}}
	d := newNameDirectory(f, nil)
	for i := 0; i < 3; i++ {
		if got := d.UserName(context.Background(), "open_id", "ou_1"); got != "Alice" {
			t.Fatalf("got %q", got)
		}
		if got := d.ChatName(context.Background(), "oc_1"); got != "Dev team" {
			t.Fatalf("got %q", got)
		}
	}
	if f.calls != 2 {
		t.Fatalf("expected one call per entity, got %d", f.calls)
	}
}

func TestMissingScopeIsLoggedOnceAndNotRetriedPerMessage(t *testing.T) {
	f := &fakeFetcher{err: errNoPermission}
	var warnings []string
	d := newNameDirectory(f, func(format string, args ...any) { warnings = append(warnings, format) })
	for i := 0; i < 5; i++ {
		if got := d.UserName(context.Background(), "user_id", "u"+string(rune('a'+i))); got != "" {
			t.Fatalf("no permission means no name, got %q", got)
		}
	}
	if f.calls != 1 {
		t.Fatalf("a missing scope must stop further lookups, got %d calls", f.calls)
	}
	if len(warnings) != 1 {
		t.Fatalf("exactly one warning expected, got %d", len(warnings))
	}
	// Chat lookups are a separate permission and still tried.
	d.ChatName(context.Background(), "oc_1")
	if f.calls != 2 {
		t.Fatalf("chat names have their own scope, got %d calls", f.calls)
	}
}

func TestPermissionIsRetriedAfterTheBackoff(t *testing.T) {
	f := &fakeFetcher{err: errNoPermission}
	d := newNameDirectory(f, nil)
	now := time.Now()
	d.now = func() time.Time { return now }
	d.UserName(context.Background(), "open_id", "ou_1")
	now = now.Add(noPermissionFor + time.Second)
	f.err = nil
	f.users = map[string]string{"ou_1": "Alice"}
	if got := d.UserName(context.Background(), "open_id", "ou_1"); got != "Alice" {
		t.Fatalf("lookups must resume once the scope is granted, got %q", got)
	}
}

func TestOtherFailuresAreBrieflyCachedAndEmptyNamesStayEmpty(t *testing.T) {
	f := &fakeFetcher{err: errors.New("boom")}
	d := newNameDirectory(f, nil)
	d.UserName(context.Background(), "open_id", "ou_1")
	d.UserName(context.Background(), "open_id", "ou_1")
	if f.calls != 1 {
		t.Fatalf("a failing lookup must not be retried per message, got %d", f.calls)
	}
	if got := d.UserName(context.Background(), "open_id", ""); got != "" || f.calls != 1 {
		t.Fatal("an empty id is never looked up")
	}
}

func TestNilDirectoryIsSafe(t *testing.T) {
	var d *nameDirectory
	if d.UserName(context.Background(), "open_id", "x") != "" || d.ChatName(context.Background(), "x") != "" {
		t.Fatal("nil directory must return no names")
	}
}

func TestAddNamesFillsSenderAndGroupTitle(t *testing.T) {
	f := &fakeFetcher{users: map[string]string{"ou_1": "Alice", "u_1": "Alice G"}, chats: map[string]string{"oc_1": "Dev team"}}
	b := &Bot{names: newNameDirectory(f, nil)}

	dm := &core.Message{Sender: core.Sender{ID: "ou_1"}, Metadata: map[string]interface{}{"chat_type": core.ChatTypeDirect, "original_chat_id": "oc_dm"}}
	b.addNames(context.Background(), dm)
	if dm.Sender.DisplayName != "Alice" || f.lastTyp != "open_id" {
		t.Fatalf("got %q via %s", dm.Sender.DisplayName, f.lastTyp)
	}
	if dm.Recipient.DisplayName != "" {
		t.Fatalf("a direct chat has no title, got %q", dm.Recipient.DisplayName)
	}

	grp := &core.Message{Sender: core.Sender{ID: "u_1"}, Metadata: map[string]interface{}{"chat_type": core.ChatTypeGroup, "original_chat_id": "oc_1", "sender_user_id": "u_1"}}
	b.addNames(context.Background(), grp)
	if grp.Sender.DisplayName != "Alice G" || f.lastTyp != "user_id" {
		t.Fatalf("a global user_id is looked up as user_id, got %q via %s", grp.Sender.DisplayName, f.lastTyp)
	}
	if grp.Recipient.DisplayName != "Dev team" {
		t.Fatalf("group title missing: %q", grp.Recipient.DisplayName)
	}

	(&Bot{}).addNames(context.Background(), grp)
	b.addNames(context.Background(), nil)
}

func TestOnePersonOutsideTheContactScopeDoesNotSwitchOffLookupsForEveryone(t *testing.T) {
	f := &fakeFetcher{users: map[string]string{"ou_ok": "Alice"}}
	d := newNameDirectory(f, nil)
	// A per-person failure (not errNoPermission) is cached for that person only.
	f.err = errors.New("feishu contact lookup: code 41050")
	d.UserName(context.Background(), "open_id", "ou_stranger")
	f.err = nil
	if got := d.UserName(context.Background(), "open_id", "ou_ok"); got != "Alice" {
		t.Fatalf("other people must still be looked up, got %q", got)
	}
	for code := range feishuPermissionCodes {
		if code == 41050 || code == 99992402 {
			t.Fatalf("code %d is per-person/field, not an app scope problem", code)
		}
	}
}

type slowFetcher struct {
	fakeFetcher
	release chan struct{}
	started chan struct{}
}

func (s *slowFetcher) userName(ctx context.Context, idType, id string) (string, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	s.started <- struct{}{}
	<-s.release
	return "Alice", nil
}

func TestConcurrentMessagesFromOnePersonShareOneLookup(t *testing.T) {
	f := &slowFetcher{release: make(chan struct{}), started: make(chan struct{}, 8)}
	d := newNameDirectory(f, nil)
	done := make(chan string, 3)
	for i := 0; i < 3; i++ {
		go func() { done <- d.UserName(context.Background(), "open_id", "ou_1") }()
	}
	<-f.started
	time.Sleep(30 * time.Millisecond) // let the other two reach the shared call
	close(f.release)
	for i := 0; i < 3; i++ {
		if got := <-done; got != "Alice" {
			t.Fatalf("got %q", got)
		}
	}
	if f.calls != 1 {
		t.Fatalf("three concurrent messages must cost one lookup, got %d", f.calls)
	}
}

func TestNameCacheIsBounded(t *testing.T) {
	f := &fakeFetcher{users: map[string]string{}}
	d := newNameDirectory(f, nil)
	for i := 0; i < maxCachedNames+50; i++ {
		d.UserName(context.Background(), "open_id", "ou_"+string(rune('a'+i%26))+string(rune('A'+(i/26)%26))+string(rune('0'+i/676)))
	}
	if len(d.cache) > maxCachedNames {
		t.Fatalf("cache grew to %d", len(d.cache))
	}
}
