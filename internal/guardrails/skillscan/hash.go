package skillscan

import (
	"crypto/sha256"
	"encoding/hex"
)

// artifactHash hashes the normalized file set so callers can cache scan results
// against actual content instead of filesystem paths.
func artifactHash(files []fileContent) string {
	hash := sha256.New()
	for _, file := range files {
		hash.Write([]byte(file.RelativePath))
		hash.Write([]byte{0})
		hash.Write([]byte(file.Content))
		hash.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}
