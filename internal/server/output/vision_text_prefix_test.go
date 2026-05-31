package output

import (
	"strings"
	"testing"
)

func TestVisionTextPrefix_FirstCallReturnsPrefix(t *testing.T) {
	v := NewVisionTextPrefix([]string{"a red apple", "a terminal screenshot"})
	got := v.PrefixText()
	want := "[Vision: a red apple; a terminal screenshot]\n\n"
	if got != want {
		t.Fatalf("first call: want %q, got %q", want, got)
	}
	if v.PrefixText() != "" {
		t.Fatal("second call must return empty (injected flag should suppress repeats)")
	}
}

func TestVisionTextPrefix_SingleDescription(t *testing.T) {
	v := NewVisionTextPrefix([]string{"a single image"})
	got := v.PrefixText()
	want := "[Vision: a single image]\n\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestVisionTextPrefix_EmptyDescriptionsAlwaysEmpty(t *testing.T) {
	for _, descs := range [][]string{nil, {}} {
		v := NewVisionTextPrefix(descs)
		if got := v.PrefixText(); got != "" {
			t.Errorf("empty descs: want \"\", got %q", got)
		}
		// Even after a call the empty-descs path stays empty.
		if got := v.PrefixText(); got != "" {
			t.Errorf("empty descs second call: want \"\", got %q", got)
		}
	}
}

func TestVisionTextPrefix_NilReceiverSafe(t *testing.T) {
	var v *VisionTextPrefix
	if got := v.PrefixText(); got != "" {
		t.Fatalf("nil receiver should return \"\", got %q", got)
	}
}

func TestVisionTextPrefix_ManyDescriptionsJoinedWithSemicolon(t *testing.T) {
	descs := []string{"alpha", "beta", "gamma", "delta"}
	v := NewVisionTextPrefix(descs)
	got := v.PrefixText()
	wantJoined := strings.Join(descs, "; ")
	if !strings.Contains(got, wantJoined) {
		t.Fatalf("want prefix containing %q, got %q", wantJoined, got)
	}
	if !strings.HasPrefix(got, "[Vision: ") {
		t.Fatalf("want '[Vision: ' prefix, got %q", got)
	}
	if !strings.HasSuffix(got, "]\n\n") {
		t.Fatalf("want ']\\n\\n' suffix, got %q", got)
	}
}
