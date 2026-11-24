package access

import "testing"

func TestUsableName(t *testing.T) {
	cases := []struct{ name, id, want string }{
		{"", "123", ""},
		{"   ", "123", ""},
		{"Alice", "123", "Alice"},
		{" Alice ", "123", "Alice"},
		{"ou_abc", "ou_abc", ""},
		{"123456789", "telegram:123456789", ""},
		{"Alice", "", "Alice"},
		{"Team", "telegram:-100", "Team"},
	}
	for _, c := range cases {
		if got := UsableName(c.name, c.id); got != c.want {
			t.Errorf("UsableName(%q,%q)=%q want %q", c.name, c.id, got, c.want)
		}
	}
}
