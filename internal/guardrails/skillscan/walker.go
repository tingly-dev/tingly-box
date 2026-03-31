package skillscan

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

var defaultScannableExtensions = []string{
	".js", ".ts", ".jsx", ".tsx", ".mjs", ".cjs",
	".py",
	".json", ".yaml", ".yml", ".toml",
	".sol",
	".sh", ".bash",
	".md",
}

// defaultIgnoredDirs keeps the first version focused on source-like content and
// avoids scanning vendored/build output that would generate noisy findings.
var defaultIgnoredDirs = map[string]struct{}{
	".git":         {},
	"node_modules": {},
	"dist":         {},
	"build":        {},
	"coverage":     {},
	"__pycache__":  {},
}

type fileContent struct {
	AbsolutePath string
	RelativePath string
	Extension    string
	Content      string
}

// walkPath normalizes file/dir input into a sorted list of in-memory file payloads.
func walkPath(path string) ([]fileContent, TargetKind, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, "", err
	}

	if !info.IsDir() {
		file, err := readFile(path, filepath.Base(path))
		if err != nil {
			return nil, "", err
		}
		return []fileContent{file}, TargetKindFile, nil
	}

	files := make([]fileContent, 0)
	err = filepath.WalkDir(path, func(current string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			if _, ignored := defaultIgnoredDirs[d.Name()]; ignored && current != path {
				return filepath.SkipDir
			}
			return nil
		}

		if !shouldScanExtension(filepath.Ext(d.Name())) {
			return nil
		}
		if isIgnoredFile(d.Name()) {
			return nil
		}

		relPath, err := filepath.Rel(path, current)
		if err != nil {
			return err
		}

		file, err := readFile(current, relPath)
		if err != nil {
			return nil
		}
		files = append(files, file)
		return nil
	})
	if err != nil {
		return nil, "", err
	}

	slices.SortFunc(files, func(a, b fileContent) int {
		return strings.Compare(a.RelativePath, b.RelativePath)
	})
	return files, TargetKindDir, nil
}

func readFile(absolutePath, relativePath string) (fileContent, error) {
	data, err := os.ReadFile(absolutePath)
	if err != nil {
		return fileContent{}, err
	}
	return fileContent{
		AbsolutePath: absolutePath,
		RelativePath: filepath.ToSlash(relativePath),
		Extension:    strings.ToLower(filepath.Ext(absolutePath)),
		Content:      string(data),
	}, nil
}

// shouldScanExtension enforces the scanner's current supported file types.
func shouldScanExtension(ext string) bool {
	ext = strings.ToLower(ext)
	return slices.Contains(defaultScannableExtensions, ext)
}

// isIgnoredFile filters bulky/generated files that are not useful skill sources.
func isIgnoredFile(name string) bool {
	switch strings.ToLower(name) {
	case "package-lock.json", "yarn.lock", "pnpm-lock.yaml":
		return true
	default:
		return strings.HasSuffix(strings.ToLower(name), ".min.js")
	}
}
