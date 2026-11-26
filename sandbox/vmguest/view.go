package vmguest

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// step is one mount of the guest's view: an empty directory (fresh), or a
// path of the share.
type step struct {
	path  string
	fresh bool
	m     Mount
}

// viewSteps are s's fresh directories and mounts in the order the guest
// mounts them: by depth, so a path lands on top of every shorter one (the
// longest prefix wins, whatever the order of the lists); a fresh directory
// before a mount at the same path.
func viewSteps(s *Spec) []step {
	var out []step
	for _, f := range s.Fresh {
		out = append(out, step{path: filepath.Clean(f), fresh: true})
	}
	for _, m := range s.Mounts {
		out = append(out, step{path: filepath.Clean(m.Path), m: m})
	}
	depth := func(p string) int { return strings.Count(p, "/") - btoi(p == "/") }
	sort.SliceStable(out, func(i, j int) bool {
		if di, dj := depth(out[i].path), depth(out[j].path); di != dj {
			return di < dj
		}
		return out[i].fresh && !out[j].fresh
	})
	return out
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// foreignCommand is the command to run for path in a guest whose system is
// not the host's: path itself when the guest can run it (an ELF executable
// or a script), else the program of its base name on PATH (a host
// executable given by its host path, such as the agent's CLI).
func foreignCommand(path string, head func(string) []byte) string {
	if !filepath.IsAbs(path) {
		return path
	}
	if h := head(path); bytes.HasPrefix(h, []byte("\x7fELF")) || bytes.HasPrefix(h, []byte("#!")) {
		return path
	}
	if p, err := exec.LookPath(filepath.Base(path)); err == nil {
		return p
	}
	return path
}

// withUser is an /etc/passwd or /etc/group with an entry for id added when
// it has none: line is the entry.
func withUser(content string, id int, line string) string {
	for _, l := range strings.Split(content, "\n") {
		if f := strings.Split(l, ":"); len(f) > 2 && f[2] == strconv.Itoa(id) {
			return content
		}
	}
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content + line + "\n"
}

// passwdLine and groupLine are the entries for the host user.
func passwdLine(name string, uid, gid int, home string) string {
	return fmt.Sprintf("%s:x:%d:%d::%s:/bin/sh", name, uid, gid, home)
}

func groupLine(name string, gid int) string { return fmt.Sprintf("%s:x:%d:", name, gid) }
