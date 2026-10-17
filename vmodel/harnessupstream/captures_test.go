package harnessupstream

import (
	"fmt"
	"net/http/httptest"
	"testing"
)

func TestCaptureLogDropsOldestPastLimit(t *testing.T) {
	l := newCaptureLog(3)
	for i := 1; i <= 5; i++ {
		r := httptest.NewRequest("POST", "/v1/messages", nil)
		r.Header.Set(RequestIDHeader, fmt.Sprintf("req-%d", i))
		l.add(r, "m", []byte(`{}`))
	}
	got := l.find("", "", 10)
	if len(got) != 3 {
		t.Fatalf("records = %d, want 3", len(got))
	}
	if got[0].RequestID != "req-5" || got[2].RequestID != "req-3" {
		t.Errorf("want newest-first req-5..req-3, got %s..%s", got[0].RequestID, got[2].RequestID)
	}
	if got[0].Seq != 5 {
		t.Errorf("seq = %d, want 5 (sequence keeps counting across drops)", got[0].Seq)
	}
}

func TestCaptureLogKeepsNonJSONBodyVerbatim(t *testing.T) {
	l := newCaptureLog(defaultCaptureLimit)
	l.add(httptest.NewRequest("POST", "/v1/messages", nil), "", []byte("not json"))
	got := l.find("", "", 1)
	if len(got) != 1 || got[0].Body != nil || got[0].RawBody != "not json" {
		t.Fatalf("got %+v, want RawBody verbatim and no Body", got)
	}
}

func TestNewRegistersEveryScenarioWithPrefix(t *testing.T) {
	ids := New().Models()
	want := map[string]bool{ModelID("text"): false, ModelID("tool_use"): false, ModelID("error-500"): false}
	for _, id := range ids {
		if _, ok := want[id]; ok {
			want[id] = true
		}
	}
	for id, seen := range want {
		if !seen {
			t.Errorf("model %s not registered; got %v", id, ids)
		}
	}
}
