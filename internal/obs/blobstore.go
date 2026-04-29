package obs

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// blobPath returns the filesystem path for a blob given its sha256 hex hash.
// Layout: {baseDir}/blobs/{hash[0:2]}/{hash[2:4]}/{hash}.json
func blobPath(baseDir, hash string) string {
	return filepath.Join(baseDir, "blobs", hash[0:2], hash[2:4], hash+".json")
}

// hashBytes returns the sha256 hex string of content.
func hashBytes(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:])
}

// writeBlob atomically writes content to the blob path using tmp+rename.
// If the blob already exists on disk (idempotent), no write is performed.
func writeBlob(baseDir, hash string, content []byte) error {
	// Validate hash to prevent path traversal.
	if len(hash) != 64 || !isHex(hash) {
		return fmt.Errorf("obs: invalid blob hash %q", hash)
	}
	dest := blobPath(baseDir, hash)
	if _, err := os.Stat(dest); err == nil {
		return nil // already on disk
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, content, 0644); err != nil {
		return err
	}
	if err := syncFile(tmp); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}

// syncFile calls Sync on a named file.
func syncFile(name string) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	err = f.Sync()
	f.Close()
	return err
}

// scanBlobSet walks the blobs directory and returns all known hashes.
func scanBlobSet(baseDir string) (map[string]struct{}, error) {
	set := make(map[string]struct{})
	blobsDir := filepath.Join(baseDir, "blobs")
	if _, err := os.Stat(blobsDir); os.IsNotExist(err) {
		return set, nil
	}
	err := filepath.WalkDir(blobsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		base := d.Name()
		if strings.HasSuffix(base, ".json") {
			hash := strings.TrimSuffix(base, ".json")
			if len(hash) == 64 && isHex(hash) {
				set[hash] = struct{}{}
			}
		}
		return nil
	})
	return set, err
}

func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
