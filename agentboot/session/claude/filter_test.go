package claude

import (
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot/session"
)

func TestDefaultSessionFilter(t *testing.T) {
	filter := DefaultSessionFilter()

	tests := []struct {
		name     string
		metadata session.SessionMetadata
		want     bool
	}{
		{
			name: "valid session with normal content",
			metadata: session.SessionMetadata{
				SessionID:    "test-1",
				FirstMessage: "Help me write a function",
			},
			want: true,
		},
		{
			name: "session with too short content",
			metadata: session.SessionMetadata{
				SessionID:    "test-2",
				FirstMessage: "hi",
			},
			want: false,
		},
		{
			name: "session with empty content",
			metadata: session.SessionMetadata{
				SessionID:    "test-3",
				FirstMessage: "",
			},
			want: false,
		},
		{
			name: "session with whitespace only",
			metadata: session.SessionMetadata{
				SessionID:    "test-4",
				FirstMessage: "   ",
			},
			want: false,
		},
		{
			name: "session with local-command-caveat pattern",
			metadata: session.SessionMetadata{
				SessionID:    "test-5",
				FirstMessage: "<local-command-caveat>Some content",
			},
			want: false,
		},
		{
			name: "session with ide_opened_file pattern",
			metadata: session.SessionMetadata{
				SessionID:    "test-6",
				FirstMessage: "<ide_opened_file>/path/to/file",
			},
			want: false,
		},
		{
			name: "session with local-command pattern",
			metadata: session.SessionMetadata{
				SessionID:    "test-7",
				FirstMessage: "<local-command-test> command",
			},
			want: false,
		},
		{
			name: "session with last user message only (valid)",
			metadata: session.SessionMetadata{
				SessionID:       "test-8",
				FirstMessage:    "",
				LastUserMessage: "This is a longer message that should pass",
			},
			want: true,
		},
		{
			name: "session with both short messages",
			metadata: session.SessionMetadata{
				SessionID:       "test-9",
				FirstMessage:    "abc",
				LastUserMessage: "xyz",
			},
			want: false,
		},
		{
			name: "session with short first but valid last message",
			metadata: session.SessionMetadata{
				SessionID:       "test-10",
				FirstMessage:    "hi",
				LastUserMessage: "This is a substantial follow-up question",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filter(tt.metadata)
			if got != tt.want {
				t.Errorf("DefaultSessionFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWithFilter(t *testing.T) {
	sessions := []session.SessionMetadata{
		{SessionID: "1", FirstMessage: "Valid session"},
		{SessionID: "2", FirstMessage: "hi"},
		{SessionID: "3", FirstMessage: "Another valid session"},
		{SessionID: "4", FirstMessage: ""},
	}

	filter := DefaultSessionFilter()
	filtered := WithFilter(sessions, filter)

	if len(filtered) != 2 {
		t.Errorf("WithFilter() returned %d sessions, want 2", len(filtered))
	}

	if filtered[0].SessionID != "1" {
		t.Errorf("First filtered session ID = %s, want 1", filtered[0].SessionID)
	}
	if filtered[1].SessionID != "3" {
		t.Errorf("Second filtered session ID = %s, want 3", filtered[1].SessionID)
	}
}

func TestWithFilterNil(t *testing.T) {
	sessions := []session.SessionMetadata{
		{SessionID: "1", FirstMessage: "Valid session"},
		{SessionID: "2", FirstMessage: "hi"},
	}

	filtered := WithFilter(sessions, nil)

	if len(filtered) != 2 {
		t.Errorf("WithFilter(nil) returned %d sessions, want 2", len(filtered))
	}
}

func TestExcludeShortContent(t *testing.T) {
	filter := ExcludeShortContent(10)

	tests := []struct {
		name     string
		metadata session.SessionMetadata
		want     bool
	}{
		{
			name: "content longer than min length",
			metadata: session.SessionMetadata{
				SessionID:    "1",
				FirstMessage: "This is a long message",
			},
			want: true,
		},
		{
			name: "content shorter than min length",
			metadata: session.SessionMetadata{
				SessionID:    "2",
				FirstMessage: "Short",
			},
			want: false,
		},
		{
			name: "last user message passes",
			metadata: session.SessionMetadata{
				SessionID:       "3",
				FirstMessage:    "Short",
				LastUserMessage: "This is a long follow-up",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filter(tt.metadata)
			if got != tt.want {
				t.Errorf("ExcludeShortContent(10)() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExcludePatterns(t *testing.T) {
	patterns := []string{"<test>", "[internal]", "/system/"}
	filter := ExcludePatterns(patterns)

	tests := []struct {
		name     string
		metadata session.SessionMetadata
		want     bool
	}{
		{
			name: "contains first pattern",
			metadata: session.SessionMetadata{
				SessionID:    "1",
				FirstMessage: "<test> command",
			},
			want: false,
		},
		{
			name: "contains second pattern",
			metadata: session.SessionMetadata{
				SessionID:    "2",
				FirstMessage: "[internal] system event",
			},
			want: false,
		},
		{
			name: "no pattern match",
			metadata: session.SessionMetadata{
				SessionID:    "3",
				FirstMessage: "Normal user message",
			},
			want: true,
		},
		{
			name: "checks last user message",
			metadata: session.SessionMetadata{
				SessionID:       "4",
				FirstMessage:    "",
				LastUserMessage: "/system/path command",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filter(tt.metadata)
			if got != tt.want {
				t.Errorf("ExcludePatterns() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCombineFilters(t *testing.T) {
	shortFilter := ExcludeShortContent(10)
	patternsFilter := ExcludePatterns([]string{"<test>"})

	combined := CombineFilters(shortFilter, patternsFilter)

	tests := []struct {
		name     string
		metadata session.SessionMetadata
		want     bool
	}{
		{
			name: "passes both filters",
			metadata: session.SessionMetadata{
				SessionID:    "1",
				FirstMessage: "Valid long message",
			},
			want: true,
		},
		{
			name: "fails short filter",
			metadata: session.SessionMetadata{
				SessionID:    "2",
				FirstMessage: "Short msg",
			},
			want: false,
		},
		{
			name: "fails pattern filter",
			metadata: session.SessionMetadata{
				SessionID:    "3",
				FirstMessage: "This is long but <test> pattern",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := combined(tt.metadata)
			if got != tt.want {
				t.Errorf("CombineFilters()() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStoreFilteredIntegration(t *testing.T) {
	// Create test sessions
	sessions := []session.SessionMetadata{
		{
			SessionID:    "valid-1",
			FirstMessage: "Help me write code",
			StartTime:    time.Now().Add(-2 * time.Hour),
		},
		{
			SessionID:    "meta-1",
			FirstMessage: "<local-command-caveat>Warning",
			StartTime:    time.Now().Add(-1 * time.Hour),
		},
		{
			SessionID:    "valid-2",
			FirstMessage: "Another valid request",
			StartTime:    time.Now(),
		},
		{
			SessionID:    "short-1",
			FirstMessage: "hi",
			StartTime:    time.Now(),
		},
	}

	// Apply default filter
	filter := DefaultSessionFilter()
	filtered := WithFilter(sessions, filter)

	// Should only have 2 valid sessions
	if len(filtered) != 2 {
		t.Errorf("Expected 2 filtered sessions, got %d", len(filtered))
	}

	// Check IDs
	ids := make(map[string]bool)
	for _, sess := range filtered {
		ids[sess.SessionID] = true
	}

	if !ids["valid-1"] {
		t.Error("valid-1 should be in filtered results")
	}
	if !ids["valid-2"] {
		t.Error("valid-2 should be in filtered results")
	}
	if ids["meta-1"] {
		t.Error("meta-1 should be excluded")
	}
	if ids["short-1"] {
		t.Error("short-1 should be excluded")
	}
}
