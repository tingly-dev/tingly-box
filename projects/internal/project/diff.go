package project

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Diff is the summary of a thread's changes for review: one entry per file,
// without their content. A file's patch is loaded on its own (DiffFile), so a
// change of any size opens at once and only what is read is sent.
type Diff struct {
	Base   string     `json:"base"`
	Branch string     `json:"branch"`
	Files  []DiffFile `json:"files"`
	// FilesOmitted counts the files past maxListedFiles that are not listed;
	// Additions and Deletions still cover them.
	FilesOmitted int    `json:"files_omitted,omitempty"`
	Additions    int    `json:"additions"`
	Deletions    int    `json:"deletions"`
	Uncommitted  string `json:"uncommitted"`
}

// DiffFile is one changed file of a Diff.
type DiffFile struct {
	Path string `json:"path"`
	// OldPath is where a renamed or copied file came from.
	OldPath string `json:"old_path,omitempty"`
	// Status is added, modified, deleted, renamed, copied or typechange.
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Binary    bool   `json:"binary,omitempty"`
	// Generated marks what nobody reviews line by line: lock files, minified
	// or generated sources, vendored code, or a path .gitattributes marks
	// linguist-generated. The viewer leaves them collapsed.
	Generated bool `json:"generated,omitempty"`
}

// FileDiff is one file's patch, loaded when its diff is opened.
type FileDiff struct {
	Path   string `json:"path"`
	Patch  string `json:"patch"`
	Binary bool   `json:"binary,omitempty"`
	// TooLarge is true when the patch is bigger than maxFilePatch: it is not
	// sent, and there is no way to ask for it. Such a file is reviewed with git.
	TooLarge bool `json:"too_large,omitempty"`
	// LinesCut counts lines longer than maxLineBytes that were shortened (a
	// minified bundle, an embedded blob): one such line can outweigh the file.
	LinesCut int `json:"lines_cut,omitempty"`
}

const (
	maxFilePatch = 512 << 10
	// maxLineBytes keeps a line's start and drops the rest.
	maxLineBytes = 2000
)

// maxListedFiles bounds the summary of a change that touches far more files
// than anyone reviews one by one.
var maxListedFiles = 10000

// BranchDiff summarizes base...HEAD of a worktree, plus the uncommitted
// changes the merge will not include.
func BranchDiff(ctx context.Context, worktree, base, branch string) (Diff, error) {
	d := Diff{Base: base, Branch: branch, Files: []DiffFile{}}
	// --raw and --numstat in one run: git computes the diff (and renames) once.
	out, err := git(ctx, worktree, "diff", "--raw", "--numstat", "-z", "-M", "--no-ext-diff", base+"...HEAD")
	if err != nil {
		return d, err
	}
	d.Files = parseDiffSummary(out)
	for _, f := range d.Files {
		d.Additions += f.Additions
		d.Deletions += f.Deletions
	}
	if len(d.Files) > maxListedFiles {
		d.FilesOmitted = len(d.Files) - maxListedFiles
		d.Files = d.Files[:maxListedFiles]
	}
	markGenerated(ctx, worktree, d.Files)
	d.Uncommitted, _ = git(ctx, worktree, "status", "--short")
	return d, nil
}

// parseDiffSummary reads `git diff --raw --numstat -z`: the raw records come
// first (":modes shas STATUS" then one path, or two for a rename or copy),
// then the numstat ones ("add\tdel\tpath", or "add\tdel\t" then the two paths).
func parseDiffSummary(out string) []DiffFile {
	toks := strings.Split(out, "\x00")
	var files []DiffFile
	byPath := map[string]int{}
	i := 0
	for ; i < len(toks) && strings.HasPrefix(toks[i], ":"); i++ {
		meta := strings.Fields(toks[i])
		if len(meta) < 5 || i+1 >= len(toks) {
			break
		}
		f := DiffFile{Status: diffStatus(meta[4])}
		if meta[4][0] == 'R' || meta[4][0] == 'C' {
			if i+2 >= len(toks) {
				break
			}
			f.OldPath, f.Path = toks[i+1], toks[i+2]
			i += 2
		} else {
			f.Path = toks[i+1]
			i++
		}
		byPath[f.Path] = len(files)
		files = append(files, f)
	}
	for ; i < len(toks); i++ {
		parts := strings.SplitN(toks[i], "\t", 3)
		if len(parts) < 3 {
			continue
		}
		p := parts[2]
		if p == "" && i+2 < len(toks) {
			p = toks[i+2]
			i += 2
		}
		idx, ok := byPath[p]
		if !ok {
			continue
		}
		if parts[0] == "-" {
			files[idx].Binary = true
			continue
		}
		files[idx].Additions, _ = strconv.Atoi(parts[0])
		files[idx].Deletions, _ = strconv.Atoi(parts[1])
	}
	return files
}

func diffStatus(code string) string {
	switch code[0] {
	case 'A':
		return "added"
	case 'D':
		return "deleted"
	case 'R':
		return "renamed"
	case 'C':
		return "copied"
	case 'T':
		return "typechange"
	}
	return "modified"
}

var generatedNames = map[string]bool{
	"package-lock.json": true, "npm-shrinkwrap.json": true, "pnpm-lock.yaml": true, "yarn.lock": true,
	"bun.lockb": true, "go.sum": true, "Cargo.lock": true, "poetry.lock": true, "uv.lock": true,
	"Pipfile.lock": true, "composer.lock": true, "Gemfile.lock": true, "packages.lock.json": true,
	"gradle.lockfile": true, "flake.lock": true, "Podfile.lock": true, "pubspec.lock": true,
}

var generatedSuffixes = []string{".min.js", ".min.css", ".map", ".pb.go", ".pb.cc", ".pb.h", "_pb2.py", "_pb2_grpc.py", ".generated.go", "_generated.go", ".designer.cs"}

var generatedDirs = map[string]bool{"vendor": true, "node_modules": true, "dist": true, ".yarn": true}

func generatedPath(p string) bool {
	if generatedNames[path.Base(p)] {
		return true
	}
	for _, s := range generatedSuffixes {
		if strings.HasSuffix(p, s) {
			return true
		}
	}
	for _, seg := range strings.Split(path.Dir(p), "/") {
		if generatedDirs[seg] {
			return true
		}
	}
	return false
}

// markGenerated flags the files nobody reads line by line, by name and by
// the repository's own .gitattributes (linguist-generated, linguist-vendored).
func markGenerated(ctx context.Context, worktree string, files []DiffFile) {
	for i := range files {
		files[i].Generated = generatedPath(files[i].Path)
	}
	if len(files) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", worktree, "check-attr", "-z", "--stdin", "linguist-generated", "linguist-vendored")
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANGUAGE=C")
	var in strings.Builder
	for _, f := range files {
		in.WriteString(f.Path)
		in.WriteByte(0)
	}
	cmd.Stdin = strings.NewReader(in.String())
	out, err := cmd.Output()
	if err != nil {
		return
	}
	index := make(map[string]int, len(files))
	for i, f := range files {
		index[f.Path] = i
	}
	toks := strings.Split(string(out), "\x00")
	for i := 0; i+2 < len(toks); i += 3 {
		if v := toks[i+2]; v == "set" || v == "true" {
			if idx, ok := index[toks[i]]; ok {
				files[idx].Generated = true
			}
		}
	}
}

// BranchFileDiff returns one file's patch of base...HEAD. A patch bigger than
// maxFilePatch is not read (git is stopped at the cap) and a binary file has no
// patch: the result says so and carries no content.
func BranchFileDiff(ctx context.Context, worktree, base, filePath, oldPath string) (FileDiff, error) {
	d := FileDiff{Path: filePath}
	if filePath == "" || strings.ContainsRune(filePath, 0) || strings.ContainsRune(oldPath, 0) {
		return d, errors.New("path is required")
	}
	args := []string{"--literal-pathspecs", "diff", "--no-color", "--no-ext-diff", "--no-textconv", "-M", base + "...HEAD", "--", filePath}
	if oldPath != "" && oldPath != filePath {
		args = append(args, oldPath)
	}
	out, over, err := gitCapped(ctx, worktree, maxFilePatch, args...)
	if err != nil {
		return d, err
	}
	if over {
		d.TooLarge = true
		return d, nil
	}
	if bytes.Contains(out, []byte("\nBinary files ")) || bytes.HasPrefix(out, []byte("Binary files ")) || bytes.Contains(out, []byte("\nGIT binary patch")) {
		d.Binary = true
		return d, nil
	}
	d.Patch, d.LinesCut = cutLongLines(string(out))
	return d, nil
}

// cutLongLines shortens every line longer than maxLineBytes to its start plus
// a "[+N chars]" marker, and says how many it cut. The cut falls on a
// character boundary.
func cutLongLines(patch string) (string, int) {
	if len(patch) <= maxLineBytes {
		return patch, 0
	}
	var b strings.Builder
	cut := 0
	for rest := patch; rest != ""; {
		line, tail := rest, ""
		if i := strings.IndexByte(rest, '\n'); i >= 0 {
			line, tail = rest[:i+1], rest[i+1:]
		}
		rest = tail
		if len(line) <= maxLineBytes+1 {
			b.WriteString(line)
			continue
		}
		body := strings.TrimSuffix(line, "\n")
		n := maxLineBytes
		for n > 0 && !utf8.RuneStart(body[n]) {
			n--
		}
		b.WriteString(body[:n])
		fmt.Fprintf(&b, " … [+%d chars]", utf8.RuneCountInString(body[n:]))
		if len(body) < len(line) {
			b.WriteByte('\n')
		}
		cut++
	}
	if cut == 0 {
		return patch, 0
	}
	return b.String(), cut
}

// gitCapped runs git and keeps at most limit bytes of its output; over reports
// that there was more, and git is stopped there instead of being read to the end.
func gitCapped(ctx context.Context, dir string, limit int, args ...string) (out []byte, over bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANGUAGE=C")
	w := &capWriter{limit: limit, cancel: cancel}
	var errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = w, &errb
	runErr := cmd.Run()
	if w.over {
		return w.buf.Bytes(), true, nil
	}
	if runErr != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = runErr.Error()
		}
		return nil, false, fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return w.buf.Bytes(), false, nil
}

type capWriter struct {
	buf    bytes.Buffer
	limit  int
	over   bool
	cancel context.CancelFunc
}

func (w *capWriter) Write(p []byte) (int, error) {
	if w.over {
		return len(p), nil
	}
	if room := w.limit - w.buf.Len(); len(p) > room {
		w.buf.Write(p[:room])
		w.over = true
		w.cancel()
		return len(p), nil
	}
	return w.buf.Write(p)
}
