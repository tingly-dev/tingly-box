package vmguest

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tingly-dev/tingly-box/sandbox"
)

func TestViewStepsMountShorterPathsFirst(t *testing.T) {
	s := &Spec{
		Fresh: []string{"/home/u", "/tmp"},
		Mounts: []Mount{
			{Source: "2", Path: "/home/u/proj/.git/config", ReadOnly: true},
			{Source: "0", Path: "/home/u/proj"},
			{Source: "1", Path: "/home/u"},
		},
	}
	var got []string
	for _, st := range viewSteps(s) {
		kind := "mount:" + st.m.Source
		if st.fresh {
			kind = "fresh"
		}
		got = append(got, st.path+" "+kind)
	}
	want := []string{"/tmp fresh", "/home/u fresh", "/home/u mount:1", "/home/u/proj mount:0", "/home/u/proj/.git/config mount:2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestForeignCommandRunsWhatTheGuestCan(t *testing.T) {
	dir := t.TempDir()
	tool := filepath.Join(dir, "claude")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	heads := map[string]string{
		"/Users/u/.local/bin/claude": "\xcf\xfa\xed\xfe", // Mach-O
		"/home/u/proj/run.sh":        "#!/b",
		"/home/u/proj/a.out":         "\x7fELF",
	}
	head := func(p string) []byte { return []byte(heads[p]) }
	for in, want := range map[string]string{
		"/Users/u/.local/bin/claude": tool, // a host executable: the guest's own
		"/home/u/proj/run.sh":        "/home/u/proj/run.sh",
		"/home/u/proj/a.out":         "/home/u/proj/a.out",
		"/opt/missing/claude":        tool, // not there at all
		"/opt/missing/nothing":       "/opt/missing/nothing",
		"claude":                     "claude", // by name already: exec finds it
	} {
		if got := foreignCommand(in, head); got != want {
			t.Errorf("foreignCommand(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWithUserAddsTheHostUserOnce(t *testing.T) {
	passwd := "root:x:0:0:root:/root:/bin/bash\nnobody:x:65534:65534::/nonexistent:/usr/sbin/nologin"
	got := withUser(passwd, 501, passwdLine("u", 501, 20, "/Users/u"))
	want := passwd + "\nu:x:501:20::/Users/u:/bin/sh\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if again := withUser(got, 501, passwdLine("u", 501, 20, "/Users/u")); again != got {
		t.Fatalf("added twice: %q", again)
	}
	if g := withUser("root:x:0:\n", 0, groupLine("u", 0)); g != "root:x:0:\n" {
		t.Fatalf("replaced an existing group: %q", g)
	}
}

func TestResultEnd(t *testing.T) {
	for _, c := range []struct {
		r    Result
		want *sandbox.ExitError
	}{
		{Result{}, nil},
		{Result{Exit: 3}, &sandbox.ExitError{Code: 3, Reason: sandbox.EndExited}},
		{Result{Signal: 9}, &sandbox.ExitError{Code: 137, Reason: sandbox.EndSignaled}},
		{Result{Signal: 9, OOM: true}, &sandbox.ExitError{Code: 137, Reason: sandbox.EndOOM}},
		{Result{Error: "no"}, &sandbox.ExitError{Code: -1, Reason: sandbox.EndFailed, Detail: "no"}},
	} {
		err := c.r.End()
		if c.want == nil {
			if err != nil {
				t.Errorf("%+v: got %v, want nil", c.r, err)
			}
			continue
		}
		if got, ok := err.(*sandbox.ExitError); !ok || *got != *c.want {
			t.Errorf("%+v: got %#v, want %#v", c.r, err, c.want)
		}
	}
}
