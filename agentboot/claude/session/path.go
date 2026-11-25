package session

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
)

// resolveProjectPath converts project path to Claude's encoded format
// /root/tingly-polish -> -root-tingly-polish
func (s *Store) resolveProjectPath(projectPath string) string {
	if projectPath == "" {
		return ""
	}

	encoded := encodeProjectPath(projectPath)
	return filepath.Join(s.projectsDir, encoded)
}

// encodeProjectPath encodes a project path for Claude's format
// This is a separate function for easier testing
// /root/tingly-polish -> -root-tingly-polish
func encodeProjectPath(projectPath string) string {
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return ProjectDirName(projectPath)
	}
	return ProjectDirName(absPath)
}

var notAlnum = regexp.MustCompile(`[^a-zA-Z0-9]`)

// projectDirMax is the longest name Claude Code keeps whole.
const projectDirMax = 200

// ProjectDirName is the directory name Claude Code keeps an absolute working
// directory's transcripts under (<config>/projects/<name>): every character
// that is not an ASCII letter or digit becomes "-" (/a/.b_c -> -a--b-c), and a
// name over 200 characters is cut there and given a hash of the path, as
// Claude Code does (Java's String.hashCode, absolute, base 36).
func ProjectDirName(absPath string) string {
	name := notAlnum.ReplaceAllString(absPath, "-")
	if len(name) <= projectDirMax {
		return name
	}
	var h int32
	for _, c := range utf16.Encode([]rune(absPath)) {
		h = h*31 + int32(c)
	}
	n := int64(h)
	if n < 0 {
		n = -n
	}
	return name[:projectDirMax] + "-" + strconv.FormatInt(n, 36)
}

// DecodeProjectPath decodes an encoded project path back to the original path
// -root-tingly-polish -> /root/tingly-polish
func DecodeProjectPath(encoded string) string {
	if encoded == "" {
		return ""
	}

	// Must start with -
	if !strings.HasPrefix(encoded, "-") {
		return encoded
	}

	// Remove leading - and replace all - with /
	decoded := strings.ReplaceAll(strings.TrimPrefix(encoded, "-"), "-", "/")
	return "/" + decoded
}
