package project

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// sharedFile confines access to the two project-shared directories, including
// through symlinks. The absolute paths in attachment notes are also accepted.
func (c *Coordinator) sharedFile(projectID, path string, list bool) ([]mcp.Content, error) {
	base := c.layout.Dir(projectID)
	if filepath.IsAbs(path) {
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return nil, err
		}
		path = rel
	}
	path = filepath.Clean(filepath.FromSlash(path))
	parts := strings.SplitN(path, string(filepath.Separator), 2)
	if parts[0] != "library" && parts[0] != "memory" {
		return nil, fmt.Errorf("path must be inside project library or memory")
	}
	rel := "."
	if len(parts) == 2 {
		rel = parts[1]
	}
	root, err := os.OpenRoot(filepath.Join(base, parts[0]))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(rel)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if list {
		if !info.IsDir() {
			return nil, fmt.Errorf("path is not a directory")
		}
		// All names, sorted, then capped: the file system's order would make
		// the same listing differ from call to call and cut a different 200.
		entries, err := f.ReadDir(-1)
		if err != nil {
			return nil, err
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		var lines []string
		for i, entry := range entries {
			if i == 200 {
				lines = append(lines, "… more entries omitted")
				break
			}
			name := entry.Name()
			if entry.IsDir() {
				name += "/"
			}
			lines = append(lines, name)
		}
		return []mcp.Content{&mcp.TextContent{Text: strings.Join(lines, "\n")}}, nil
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("path is not a regular file")
	}
	const maxImage = 5 << 20
	if info.Size() > maxImage {
		return nil, fmt.Errorf("file exceeds 5 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxImage+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxImage {
		return nil, fmt.Errorf("file exceeds 5 MiB")
	}
	mime := http.DetectContentType(data)
	if mime == "image/png" || mime == "image/jpeg" || mime == "image/gif" || mime == "image/webp" {
		return []mcp.Content{&mcp.ImageContent{Data: data, MIMEType: mime}}, nil
	}
	if !utf8.Valid(data) || strings.ContainsRune(string(data), 0) {
		return nil, fmt.Errorf("file is not UTF-8 text or a supported image")
	}
	const maxText = 256 << 10
	if len(data) > maxText {
		return nil, fmt.Errorf("text file exceeds 256 KiB")
	}
	return []mcp.Content{&mcp.TextContent{Text: string(data)}}, nil
}
