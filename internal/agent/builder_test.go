package agent

import "testing"

func TestCollectCodexModels_Dedup(t *testing.T) {
	got := CollectCodexModels([]string{"a", "b", "a", "c", "b"})
	want := []string{"a", "b", "c"}
	assertStringSlice(t, got, want)
}

func TestCollectCodexModels_OrderPreserved(t *testing.T) {
	got := CollectCodexModels([]string{"z", "a", "m"})
	want := []string{"z", "a", "m"}
	assertStringSlice(t, got, want)
}

func TestCollectCodexModels_WhitespaceTrimmed(t *testing.T) {
	got := CollectCodexModels([]string{"  a  ", "b", " a"})
	want := []string{"a", "b"}
	assertStringSlice(t, got, want)
}

func TestCollectCodexModels_EmptyStringsSkipped(t *testing.T) {
	got := CollectCodexModels([]string{"", "a", "  ", "b", ""})
	want := []string{"a", "b"}
	assertStringSlice(t, got, want)
}

func TestCollectCodexModels_EmptyInput(t *testing.T) {
	if got := CollectCodexModels(nil); len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
	if got := CollectCodexModels([]string{}); len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestCollectCodexModels_AllDuplicates(t *testing.T) {
	got := CollectCodexModels([]string{"x", "x", "x"})
	want := []string{"x"}
	assertStringSlice(t, got, want)
}

func assertStringSlice(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d: got %v, want %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] got %q want %q", i, got[i], want[i])
		}
	}
}
