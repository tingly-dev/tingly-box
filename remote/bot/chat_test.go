package bot_test

import (
	"fmt"
	"testing"

	"github.com/tingly-dev/tingly-box/remote/bot"
)

// ---------- project-history MRU ----------
//
// PushProjectHistory is a method on *bot.Chat; these exercise its prepend /
// dedupe / seed / cap behavior. They moved here from internal/data/db when
// PushProjectHistory moved with the Chat type.

func TestPushProjectHistory_PrependsAndDedupes(t *testing.T) {
	chat := &bot.Chat{}
	chat.PushProjectHistory("/a")
	chat.PushProjectHistory("/b")
	chat.PushProjectHistory("/c")
	chat.PushProjectHistory("/a") // dedupe — should move to front, not duplicate
	want := []string{"/a", "/c", "/b"}
	if len(chat.ProjectHistory) != len(want) {
		t.Fatalf("history length %d, want %d (%v)", len(chat.ProjectHistory), len(want), chat.ProjectHistory)
	}
	for i, w := range want {
		if chat.ProjectHistory[i] != w {
			t.Errorf("history[%d] = %q, want %q (%v)", i, chat.ProjectHistory[i], w, chat.ProjectHistory)
		}
	}
	if chat.ProjectPath != "/a" {
		t.Errorf("ProjectPath = %q, want /a", chat.ProjectPath)
	}
}

func TestPushProjectHistory_SeedsLegacyProjectPath(t *testing.T) {
	chat := &bot.Chat{ProjectPath: "/legacy"} // pre-existing binding from before history
	chat.PushProjectHistory("/new")
	want := []string{"/new", "/legacy"}
	if len(chat.ProjectHistory) != 2 || chat.ProjectHistory[0] != want[0] || chat.ProjectHistory[1] != want[1] {
		t.Errorf("history = %v, want %v", chat.ProjectHistory, want)
	}
}

func TestRememberFolder_LeavesProjectPathAlone(t *testing.T) {
	chat := &bot.Chat{ProjectPath: "/legacy"}
	chat.RememberFolder("/a")
	chat.RememberFolder("/b")
	want := []string{"/b", "/a", "/legacy"}
	if len(chat.ProjectHistory) != len(want) {
		t.Fatalf("history = %v, want %v", chat.ProjectHistory, want)
	}
	for i, w := range want {
		if chat.ProjectHistory[i] != w {
			t.Errorf("history[%d] = %q, want %q (%v)", i, chat.ProjectHistory[i], w, chat.ProjectHistory)
		}
	}
	if chat.ProjectPath != "/legacy" {
		t.Errorf("ProjectPath = %q, want it untouched (/legacy)", chat.ProjectPath)
	}
}

func TestPushProjectHistory_EmptyPathIsNoOp(t *testing.T) {
	chat := &bot.Chat{ProjectPath: "/x", ProjectHistory: []string{"/x"}}
	chat.PushProjectHistory("")
	if chat.ProjectPath != "/x" || len(chat.ProjectHistory) != 1 {
		t.Errorf("empty path should not mutate state: path=%q history=%v", chat.ProjectPath, chat.ProjectHistory)
	}
}

func TestPushProjectHistory_Caps(t *testing.T) {
	chat := &bot.Chat{}
	for i := 0; i < bot.ProjectHistoryCap+5; i++ {
		chat.PushProjectHistory(fmt.Sprintf("/p%d", i))
	}
	if len(chat.ProjectHistory) != bot.ProjectHistoryCap {
		t.Errorf("history not capped: got %d, want %d", len(chat.ProjectHistory), bot.ProjectHistoryCap)
	}
}
