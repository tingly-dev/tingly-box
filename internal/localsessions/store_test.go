package localsessions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, body string) (*Store, string, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "-work"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "-work", "session.jsonl")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	return s, s.encode(token{Kind: "session", Rel: filepath.Join("-work", "session.jsonl")}), path
}
func line(id string) string {
	return fmt.Sprintf("{\"type\":\"user\",\"uuid\":%q,\"message\":{\"role\":\"user\",\"content\":%q}}\n", id, id)
}
func TestPreviewPagination(t *testing.T) {
	var body strings.Builder
	for i := 0; i < 125; i++ {
		body.WriteString(line(fmt.Sprintf("m%03d", i)))
	}
	s, id, _ := fixture(t, body.String())
	for _, direction := range []string{"forward", "backward"} {
		t.Run(direction, func(t *testing.T) {
			cursor := ""
			seen := map[string]bool{}
			pages := 0
			for {
				page, err := s.Preview(context.Background(), id, cursor, direction)
				if err != nil {
					t.Fatal(err)
				}
				pages++
				if len(page.Events) > MaxEvents || page.ScannedBytes > MaxReadBytes {
					t.Fatal("unbounded page")
				}
				wire, _ := json.Marshal(page)
				if len(wire) > 256<<10 {
					t.Fatal("oversized response")
				}
				for i, event := range page.Events {
					if seen[event.ID] {
						t.Fatalf("duplicate %s", event.ID)
					}
					seen[event.ID] = true
					if i > 0 && page.Events[i-1].ID > event.ID {
						t.Fatal("page is not in transcript order")
					}
				}
				if page.NextCursor == "" {
					break
				}
				if page.NextCursor == cursor {
					t.Fatal("cursor did not advance")
				}
				cursor = page.NextCursor
			}
			if len(seen) != 125 || pages != 3 {
				t.Fatalf("got %d events in %d pages", len(seen), pages)
			}
		})
	}
}
func TestLargeSparseTranscriptReadsOnlyWindow(t *testing.T) {
	s, id, path := fixture(t, line("first"))
	f, err := os.OpenFile(path, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Seek(1<<30, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString("\n" + line("last")); err != nil {
		t.Fatal(err)
	}
	f.Close()
	page, err := s.Preview(context.Background(), id, "", "backward")
	if err != nil {
		t.Fatal(err)
	}
	if page.FileSize < 1<<30 || page.ScannedBytes > MaxReadBytes || len(page.Events) != 1 || page.Events[0].ID != "last" || page.NextCursor == "" {
		t.Fatalf("unexpected preview: size=%d scanned=%d events=%v", page.FileSize, page.ScannedBytes, page.Events)
	}
	next, err := s.Preview(context.Background(), id, page.NextCursor, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Events) != 0 || next.NextCursor == page.NextCursor || next.ScannedBytes > MaxReadBytes {
		t.Fatal("oversized line did not advance within budget")
	}
	head, err := s.Preview(context.Background(), id, "", "forward")
	if err != nil || len(head.Events) != 1 || head.Events[0].ID != "first" {
		t.Fatalf("head failed: %v", err)
	}
}
func TestOversizedLineAcrossWindows(t *testing.T) {
	body := line("first") + `{"type":"user","message":{"content":"` + strings.Repeat("x", 3<<20) + `"}}` + "\n" + line("last")
	s, id, _ := fixture(t, body)
	for _, direction := range []string{"forward", "backward"} {
		cursor := ""
		seen := map[string]bool{}
		for count := 0; count < 5; count++ {
			page, err := s.Preview(context.Background(), id, cursor, direction)
			if err != nil {
				t.Fatal(err)
			}
			for _, event := range page.Events {
				seen[event.ID] = true
			}
			if page.NextCursor == "" {
				break
			}
			cursor = page.NextCursor
		}
		if len(seen) != 2 || !seen["first"] || !seen["last"] {
			t.Fatalf("%s: %v", direction, seen)
		}
	}
}
func TestVersionedCursorRejectsChanges(t *testing.T) {
	for _, change := range []string{"append", "truncate", "replace"} {
		t.Run(change, func(t *testing.T) {
			s, id, path := fixture(t, strings.Repeat(line("one"), 60))
			page, err := s.Preview(context.Background(), id, "", "backward")
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "append":
				f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
				f.WriteString(line("new"))
				f.Close()
			case "truncate":
				os.Truncate(path, 10)
			case "replace":
				os.Remove(path)
				os.WriteFile(path, []byte(line("replacement")), 0600)
			}
			if _, err := s.Preview(context.Background(), id, page.NextCursor, ""); !errors.Is(err, ErrChanged) {
				t.Fatalf("got %v", err)
			}
		})
	}
}
func TestCursorReplayAndScope(t *testing.T) {
	s, id, _ := fixture(t, strings.Repeat(line("one"), 60))
	page, err := s.Preview(context.Background(), id, "", "backward")
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.Preview(context.Background(), id, page.NextCursor, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Preview(context.Background(), id, page.NextCursor, "")
	if err != nil {
		t.Fatal(err)
	}
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	if string(aj) != string(bj) {
		t.Fatal("cursor retry changed its page")
	}
	other := s.encode(token{Kind: "session", Rel: "-work/other.jsonl"})
	os.WriteFile(filepath.Join(s.root, "-work", "other.jsonl"), []byte(line("other")), 0600)
	if _, err := s.Preview(context.Background(), other, page.NextCursor, ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cross-session cursor: %v", err)
	}
	for _, bad := range []string{"../../etc/passwd", id + "x", s.encode(token{Kind: "session", Rel: "../outside.jsonl"})} {
		if _, err := s.Preview(context.Background(), bad, "", "backward"); err == nil {
			t.Fatal("accepted invalid identifier")
		}
	}
}
func TestListDoesNotParseWholeFiles(t *testing.T) {
	s, _, _ := fixture(t, line("first"))
	for i := 0; i < 30; i++ {
		if err := os.WriteFile(filepath.Join(s.root, "-work", fmt.Sprintf("%d.jsonl", i)), []byte(line("title")), 0600); err != nil {
			t.Fatal(err)
		}
	}
	projects, err := s.List(context.Background(), "", "")
	if err != nil || len(projects.Entries) != 1 {
		t.Fatalf("projects: %v", err)
	}
	page, err := s.List(context.Background(), projects.Entries[0].ID, "")
	if err != nil || len(page.Entries) != 25 || page.Total != 31 || page.NextCursor == "" {
		t.Fatalf("listing: %v, %d entries", err, len(page.Entries))
	}
	second, err := s.List(context.Background(), projects.Entries[0].ID, page.NextCursor)
	if err != nil || len(second.Entries) != 6 {
		t.Fatalf("next page: %v", err)
	}
	for _, entry := range page.Entries {
		if entry.Title == "" {
			t.Fatal("missing bounded title")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.List(ctx, "", ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}
func TestLinksAndNonRegularFilesRejected(t *testing.T) {
	s, _, _ := fixture(t, line("first"))
	outside := filepath.Join(t.TempDir(), "secret.jsonl")
	os.WriteFile(outside, []byte(line("secret")), 0600)
	if err := os.Symlink(outside, filepath.Join(s.root, "-work", "link.jsonl")); err != nil {
		t.Skip(err)
	}
	id := s.encode(token{Kind: "session", Rel: "-work/link.jsonl"})
	if _, err := s.Preview(context.Background(), id, "", "backward"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("symlink: %v", err)
	}
	os.Mkdir(filepath.Join(s.root, "-work", "directory.jsonl"), 0700)
	id = s.encode(token{Kind: "session", Rel: "-work/directory.jsonl"})
	if _, err := s.Preview(context.Background(), id, "", "backward"); err == nil {
		t.Fatal("accepted directory as transcript")
	}
}
func TestProjectionOmitsAttachmentsAndBoundsText(t *testing.T) {
	input := `{"type":"assistant","uuid":"a","isSidechain":true,"parentUuid":"p","message":{"role":"assistant","content":[{"type":"text","text":"<script>alert(1)</script>"},{"type":"image","source":{"data":"BASE64_SECRET"}},{"type":"tool_use","name":"Bash","input":{"command":"rm -rf /"}}]}}`
	event, ok := projectEvent([]byte(input), 0)
	if !ok || !event.Sidechain || event.ParentID != "p" {
		t.Fatal("lost transcript metadata")
	}
	wire, _ := json.Marshal(event)
	if strings.Contains(string(wire), "BASE64_SECRET") {
		t.Fatal("attachment payload leaked")
	}
	if event.Blocks[0].Text != "<script>alert(1)</script>" || event.Blocks[2].Kind != "tool_use" {
		t.Fatal("content was not projected as inert text")
	}
	s, id, _ := fixture(t, `{"type":"user","message":{"content":`+fmt.Sprintf("%q", strings.Repeat("x", 20000))+"}}\n")
	page, err := s.Preview(context.Background(), id, "", "backward")
	if err != nil || len(page.Events) != 1 || !page.Events[0].Blocks[0].Truncated || len(page.Events[0].Blocks[0].Text) > MaxBlockText+3 {
		t.Fatal("long text not bounded")
	}
}

func TestControlHeavyContentStillAdvances(t *testing.T) {
	blocks := make([]map[string]string, 8)
	for i := range blocks {
		blocks[i] = map[string]string{"type": "text", "text": strings.Repeat("\x00", 4096)}
	}
	raw, _ := json.Marshal(map[string]any{"type": "assistant", "message": map[string]any{"role": "assistant", "content": blocks}})
	s, id, _ := fixture(t, string(raw)+"\n")
	page, err := s.Preview(context.Background(), id, "", "backward")
	if err != nil || len(page.Events) != 1 || page.Events[0].Blocks[0].Label != "event_too_large" {
		t.Fatalf("unbounded event did not advance: %v", err)
	}
	wire, _ := json.Marshal(page)
	if len(wire) > 256<<10 {
		t.Fatal("response budget exceeded")
	}
}
func TestIndexPathsAreNeverFollowed(t *testing.T) {
	s, _, _ := fixture(t, line("safe"))
	outside := filepath.Join(t.TempDir(), "secret.jsonl")
	os.WriteFile(outside, []byte(line("secret")), 0600)
	index, _ := json.Marshal(map[string]any{"originalPath": "/a/display-only/path", "entries": []any{map[string]any{"fullPath": outside}}})
	os.WriteFile(filepath.Join(s.root, "-work", "sessions-index.json"), index, 0600)
	projects, err := s.List(context.Background(), "", "")
	if err != nil || projects.Entries[0].Title != "/a/display-only/path" {
		t.Fatalf("project label: %v", err)
	}
	sessions, err := s.List(context.Background(), projects.Entries[0].ID, "")
	if err != nil || len(sessions.Entries) != 1 || sessions.Entries[0].Title != "safe" {
		t.Fatalf("followed index content: %v", err)
	}
}
func BenchmarkPreviewLarge(b *testing.B) {
	root := b.TempDir()
	os.Mkdir(filepath.Join(root, "project"), 0700)
	f, err := os.Create(filepath.Join(root, "project", "s.jsonl"))
	if err != nil {
		b.Fatal(err)
	}
	f.Seek(1<<30, 0)
	f.WriteString("\n" + line("latest"))
	f.Close()
	s, err := New(root)
	if err != nil {
		b.Fatal(err)
	}
	id := s.encode(token{Kind: "session", Rel: "project/s.jsonl"})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		page, err := s.Preview(context.Background(), id, "", "backward")
		if err != nil || page.ScannedBytes > MaxReadBytes {
			b.Fatal(err)
		}
	}
}

func TestMalformedRecordsAreSkippedAndCachesStayBounded(t *testing.T) {
	s, id, _ := fixture(t, line("visible")+"{broken json}\n"+`{"type":"file-history-snapshot","payload":"not a chat message"}`+"\n")
	for i := 0; i < 70; i++ {
		page, err := s.Preview(context.Background(), id, "", "backward")
		if err != nil || len(page.Events) != 1 || page.SkippedRecords != 2 {
			t.Fatalf("malformed record handling: %v", err)
		}
	}
	if len(s.snapshots) > 64 {
		t.Fatal("snapshot cache grew beyond its bound")
	}
	for i := 0; i < 8; i++ {
		if _, err := s.List(context.Background(), "", ""); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.indexes) > 4 {
		t.Fatal("directory cache grew beyond its bound")
	}
}
func TestDirectoryEntryBudget(t *testing.T) {
	s, _, _ := fixture(t, line("visible"))
	for i := 0; i < MaxDirectoryEntries; i++ {
		f, err := os.Create(filepath.Join(s.root, "-work", fmt.Sprintf("entry-%d", i)))
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	project := s.encode(token{Kind: "project", Rel: "-work"})
	if _, err := s.List(context.Background(), project, ""); !errors.Is(err, ErrLimit) {
		t.Fatalf("unbounded directory accepted: %v", err)
	}
}

func TestMissingHistoryIsAnEmptyProjectList(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "not-created"))
	if err != nil {
		t.Fatal(err)
	}
	page, err := s.List(context.Background(), "", "")
	if err != nil || len(page.Entries) != 0 {
		t.Fatalf("missing history: %v", err)
	}
}

func TestManyTinyContentBlocksDoNotExpandIntoAnUnboundedSlice(t *testing.T) {
	raw := `{"type":"user","message":{"content":[` + strings.Repeat("{},", 100000) + `{}]}}`
	event, ok := projectEvent([]byte(raw), 0)
	if !ok || len(event.Blocks) != MaxBlocks+1 || event.Blocks[MaxBlocks].Label != "additional_blocks_omitted" {
		t.Fatal("content block budget not enforced")
	}
}
