package project

import (
	"path/filepath"
	"strings"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// addDirectories adds dirs to doc's permissions.additionalDirectories, keeping
// what the host's settings already list and skipping duplicates.
func addDirectories(doc map[string]any, dirs ...string) {
	perms := permissionsOf(doc)
	list, _ := perms["additionalDirectories"].([]any)
	list = appendUnique(list, dirs...)
	perms["additionalDirectories"] = list
}

// grantLibrary makes the project library, the shared upload directory, a place
// a thread can read and write without asking. additionalDirectories (see
// addDirectories) only widens what Claude Code's file tools may reach in
// acceptEdits mode; the explicit rules make Read/Edit work in every mode, and
// the autoMode entries tell the auto-mode classifier the directory is a
// sanctioned destination, so it stops denying a copy into it.
func grantLibrary(doc map[string]any, library string) {
	pattern := rulePath(library) + "/**"
	perms := permissionsOf(doc)
	allow, _ := perms["allow"].([]any)
	perms["allow"] = appendUnique(allow, "Read("+pattern+")", "Edit("+pattern+")")

	auto, _ := doc["autoMode"].(map[string]any)
	if auto == nil {
		auto = map[string]any{}
	}
	env, _ := auto["environment"].([]any)
	auto["environment"] = appendClassifierRule(env,
		"The project library "+library+" is the shared upload directory of this project: users' uploads and the files threads send to the conversation live there, and it is meant to be written to.")
	allow2, _ := auto["allow"].([]any)
	auto["allow"] = appendClassifierRule(allow2,
		"Creating, writing, copying or moving files into "+library+" (the shared upload directory) is allowed; it is how a thread saves an artifact such as a screenshot or report to send to the user.")
	doc["autoMode"] = auto
}

func permissionsOf(doc map[string]any) map[string]any {
	perms, _ := doc["permissions"].(map[string]any)
	if perms == nil {
		perms = map[string]any{}
		doc["permissions"] = perms
	}
	return perms
}

// rulePath is an absolute path in permission-rule syntax: "//" + path from the
// filesystem root.
func rulePath(p string) string {
	p = filepath.ToSlash(p)
	if strings.HasPrefix(p, "/") {
		return "/" + p
	}
	return p
}

func appendUnique(list []any, vals ...string) []any {
	for _, v := range vals {
		found := false
		for _, e := range list {
			if s, ok := e.(string); ok && s == v {
				found = true
				break
			}
		}
		if !found {
			list = append(list, v)
		}
	}
	return list
}

// appendClassifierRule adds a rule to an autoMode list. An empty list would
// replace the classifier's built-in rules, so it starts with "$defaults".
func appendClassifierRule(list []any, rule string) []any {
	if len(list) == 0 {
		list = []any{"$defaults"}
	}
	return appendUnique(list, rule)
}

// threadOutputDir is where a thread saves what it wants to show the user.
func threadOutputDir(l Layout, p *db.Project, t *db.Thread) string {
	return l.OutputDir(p.ID, slugify(t.Title))
}

// ArtifactGuidance tells a thread where to put files it wants the user to see:
// send_file only reads from the working directory and the library, and writes
// to the library are pre-approved while copies in from elsewhere (/tmp) are not.
func ArtifactGuidance(l Layout, p *db.Project, t *db.Thread) string {
	return "Do not use /tmp: your TMPDIR is " + l.TmpDir(p.ID, t.ID) + " (inside the project library, writable, removed when the thread is resolved); put scratch files there, and send_file can send them from there without a copy. " +
		"Save screenshots, reports and any other file you want to show the user directly under " + threadOutputDir(l, p, t) +
		" (the shared upload directory; writing there is pre-approved), not in /tmp, then pass that path to send_file. send_file only accepts files inside your working directory or the project library " + l.LibraryDir(p.ID) + "."
}

// setEnv sets key=value pairs (alternating) in env, replacing existing entries
// of the same key, without changing the slice passed in.
func setEnv(env []string, kv ...string) []string {
	out := make([]string, 0, len(env)+len(kv)/2)
	set := map[string]bool{}
	for i := 0; i+1 < len(kv); i += 2 {
		set[kv[i]] = true
	}
	for _, e := range env {
		if k, _, ok := strings.Cut(e, "="); ok && set[k] {
			continue
		}
		out = append(out, e)
	}
	for i := 0; i+1 < len(kv); i += 2 {
		out = append(out, kv[i]+"="+kv[i+1])
	}
	return out
}
