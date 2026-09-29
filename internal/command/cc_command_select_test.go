package command

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestMatchProfileInput(t *testing.T) {
	profiles := []typ.ProfileMeta{
		{ID: "p1", Name: "Work"},
		{ID: "p2", Name: "Personal"},
		{ID: "p3", Name: "Work-Alt"},
	}
	// After deleting p1/p2, list position must not be confused with ID.
	sparse := []typ.ProfileMeta{{ID: "p3", Name: "A"}, {ID: "p7", Name: "B"}}
	if got, err := matchProfileInput(sparse, "7"); err != nil || got != "p7" {
		t.Errorf("7 on sparse: got (%q, %v), want p7", got, err)
	}
	if _, err := matchProfileInput(sparse, "1"); err == nil {
		t.Errorf("1 on sparse: want error (position must not match)")
	}
	// Pure digits are strict: no fuzzy fallback onto names/IDs containing them.
	fuzzy := []typ.ProfileMeta{{ID: "p3", Name: "team-42"}, {ID: "p7", Name: "B"}}
	if _, err := matchProfileInput(fuzzy, "42"); err == nil {
		t.Errorf("42: want error, digits must match pN strictly")
	}
	cases := []struct {
		in, want string
		wantErr  bool
	}{
		{"1", "p1", false},
		{"2", "p2", false},
		{"4", "", true},
		{"9", "", true},
		{"p3", "p3", false},
		{"P2", "p2", false},
		{"work", "p1", false}, // exact name beats prefix
		{"pers", "p2", false}, // unique prefix
		{"alt", "p3", false},  // unique substring
		{"wor", "", true},     // ambiguous
		{"nope", "", true},
	}
	for _, c := range cases {
		got, err := matchProfileInput(profiles, c.in)
		if (err != nil) != c.wantErr || got != c.want {
			t.Errorf("%q: got (%q, %v), want (%q, err=%v)", c.in, got, err, c.want, c.wantErr)
		}
	}
}
