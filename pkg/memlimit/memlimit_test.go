package memlimit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLimit(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"1073741824\n", 1 << 30, true},
		{"max\n", 0, false},
		{"", 0, false},
		{"garbage", 0, false},
		{"-5", 0, false},
		{"0", 0, false},
		// cgroup v1 "unlimited" sentinel (page-rounded ~2^63)
		{"9223372036854771712", 0, false},
	}
	for _, c := range cases {
		got, ok := parseLimit(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("parseLimit(%q) = (%d, %v), want (%d, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestReadCgroupLimit(t *testing.T) {
	dir := t.TempDir()
	v2 := filepath.Join(dir, "memory.max")
	v1 := filepath.Join(dir, "memory.limit_in_bytes")

	// No files readable → not ok.
	if _, ok := readCgroupLimit([]string{v2, v1}); ok {
		t.Fatal("expected no limit when no files exist")
	}

	// v2 "max" (unlimited) falls through to v1.
	if err := os.WriteFile(v2, []byte("max\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(v1, []byte("536870912\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok := readCgroupLimit([]string{v2, v1})
	if !ok || got != 512<<20 {
		t.Fatalf("readCgroupLimit = (%d, %v), want (%d, true)", got, ok, int64(512<<20))
	}

	// v2 with a real limit wins.
	if err := os.WriteFile(v2, []byte("1073741824\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok = readCgroupLimit([]string{v2, v1})
	if !ok || got != 1<<30 {
		t.Fatalf("readCgroupLimit = (%d, %v), want (%d, true)", got, ok, int64(1<<30))
	}
}

func TestSetFromCgroupRespectsEnv(t *testing.T) {
	t.Setenv("GOMEMLIMIT", "512MiB")
	if _, ok := SetFromCgroup(); ok {
		t.Fatal("SetFromCgroup must be a no-op when GOMEMLIMIT is set")
	}
}
