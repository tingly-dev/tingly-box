package obs

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveOpts controls how missing blobs are handled during resolution.
type ResolveOpts struct {
	// AllowMissingBlobs replaces missing blob refs with {"_missing_blob":"sha256:..."}
	// instead of returning an error.
	AllowMissingBlobs bool
}

// ResolveRecord expands all {"$ref":"sha256:<hash>"} markers in a SlimRecord
// by loading the corresponding blobs from baseDir.
func ResolveRecord(baseDir string, slim *SlimRecord, opts ResolveOpts) (*SlimRecord, error) {
	out := *slim // shallow copy
	var err error

	if slim.OriginalRequest != nil {
		out.OriginalRequest, err = resolveHTTPData(baseDir, slim.OriginalRequest, opts)
		if err != nil {
			return nil, err
		}
	}
	if slim.TransformedRequest != nil {
		out.TransformedRequest, err = resolveHTTPData(baseDir, slim.TransformedRequest, opts)
		if err != nil {
			return nil, err
		}
	}
	if slim.ProviderResponse != nil {
		out.ProviderResponse, err = resolveHTTPData(baseDir, slim.ProviderResponse, opts)
		if err != nil {
			return nil, err
		}
	}
	if slim.FinalResponse != nil {
		out.FinalResponse, err = resolveHTTPData(baseDir, slim.FinalResponse, opts)
		if err != nil {
			return nil, err
		}
	}
	return &out, nil
}

func resolveHTTPData(baseDir string, d *SlimHTTPData, opts ResolveOpts) (*SlimHTTPData, error) {
	out := *d
	resolved, err := resolveValue(baseDir, d.Body, opts)
	if err != nil {
		return nil, err
	}
	out.Body = resolved
	return &out, nil
}

// resolveValue recursively expands $ref markers in v.
func resolveValue(baseDir string, v interface{}, opts ResolveOpts) (interface{}, error) {
	switch val := v.(type) {
	case map[string]interface{}:
		if ref, ok := val["$ref"].(string); ok && strings.HasPrefix(ref, "sha256:") {
			hash := strings.TrimPrefix(ref, "sha256:")
			return loadBlob(baseDir, hash, opts)
		}
		// Recurse into the map.
		out := make(map[string]interface{}, len(val))
		for k, mv := range val {
			resolved, err := resolveValue(baseDir, mv, opts)
			if err != nil {
				return nil, err
			}
			out[k] = resolved
		}
		return out, nil
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, elem := range val {
			resolved, err := resolveValue(baseDir, elem, opts)
			if err != nil {
				return nil, err
			}
			out[i] = resolved
		}
		return out, nil
	}
	return v, nil
}

func loadBlob(baseDir, hash string, opts ResolveOpts) (interface{}, error) {
	if !isHex(hash) || len(hash) != 64 {
		return nil, fmt.Errorf("obs: invalid blob hash %q", hash)
	}
	path := blobPath(baseDir, hash)
	data, err := os.ReadFile(path)
	if err != nil {
		if opts.AllowMissingBlobs {
			return map[string]string{"_missing_blob": "sha256:" + hash}, nil
		}
		return nil, fmt.Errorf("obs: blob %s not found: %w", hash[:8], err)
	}
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("obs: corrupt blob %s: %w", hash[:8], err)
	}
	return v, nil
}

// SessionIterator iterates over SlimRecords in a session JSONL file.
type SessionIterator struct {
	scanner *bufio.Scanner
	path    string
	err     error
}

// WalkSession opens the JSONL file for the given scenario and session short ID
// under baseDir and returns an iterator.
func WalkSession(baseDir, scenario, sessionShort string) (*SessionIterator, error) {
	// Find all date directories that contain a file for this session.
	// For simplicity, scan all date dirs.
	sessionsRoot := filepath.Join(baseDir, scenario, "sessions")
	pattern := filepath.Join(sessionsRoot, "*", sessionShort+".jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return nil, fmt.Errorf("obs: no session files found for %s/%s", scenario, sessionShort)
	}
	// Use the first (oldest) match; callers can iterate across dates themselves.
	f, err := os.Open(matches[0])
	if err != nil {
		return nil, err
	}
	return &SessionIterator{scanner: bufio.NewScanner(f), path: matches[0]}, nil
}

// Next advances the iterator and returns the next SlimRecord.
// Returns (nil, nil) at EOF.
func (it *SessionIterator) Next() (*SlimRecord, error) {
	if it.err != nil {
		return nil, it.err
	}
	if !it.scanner.Scan() {
		return nil, it.scanner.Err()
	}
	var s SlimRecord
	if err := json.Unmarshal(it.scanner.Bytes(), &s); err != nil {
		return nil, fmt.Errorf("obs: malformed JSONL in %s: %w", it.path, err)
	}
	return &s, nil
}
