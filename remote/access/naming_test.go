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

func TestPlatformAliasUpdate(t *testing.T) {
	cases := []struct {
		desc                string
		alias, source, name string
		wantAlias, wantSrc  string
		wantChanged         bool
	}{
		{"empty name never wipes platform alias", "Alice", AliasSourcePlatform, "", "Alice", AliasSourcePlatform, false},
		{"empty name never wipes user alias", "Boss", AliasSourceUser, "", "Boss", AliasSourceUser, false},
		{"id-echo name is no name", "Alice", AliasSourcePlatform, "ou_x", "Alice", AliasSourcePlatform, false},
		{"platform name fills empty alias", "", "", "Alice", "Alice", AliasSourcePlatform, true},
		{"platform name refreshes platform alias", "Alice", AliasSourcePlatform, "Alice B", "Alice B", AliasSourcePlatform, true},
		{"user alias is never overwritten", "Boss", AliasSourceUser, "Alice", "Boss", AliasSourceUser, false},
		{"same platform name is a no-op", "Alice", AliasSourcePlatform, "Alice", "Alice", AliasSourcePlatform, false},
		{"legacy alias without source is replaceable only when empty", "", "", "Alice", "Alice", AliasSourcePlatform, true},
	}
	for _, c := range cases {
		alias, src, changed := PlatformAliasUpdate(c.alias, c.source, c.name, "ou_x")
		if alias != c.wantAlias || src != c.wantSrc || changed != c.wantChanged {
			t.Errorf("%s: got (%q,%q,%v) want (%q,%q,%v)", c.desc, alias, src, changed, c.wantAlias, c.wantSrc, c.wantChanged)
		}
	}
}
