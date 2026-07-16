package protocoltest

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	requestrecord "github.com/tingly-dev/tingly-box/internal/record"
)

// readPersistedRequestRecordArtifacts loads the additive request_record
// envelopes emitted by the real recording sink. Keeping this reader in the
// harness package lets CLI validation inspect the exact persisted artifact
// instead of relying on an in-memory recorder snapshot.
func readPersistedRequestRecordArtifacts(root string) ([]*requestrecord.RequestRecord, error) {
	var records []*requestrecord.RequestRecord
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".jsonl.gz") {
			return nil
		}
		loaded, err := readRequestRecordArtifactFile(path)
		if err != nil {
			return err
		}
		records = append(records, loaded...)
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	return records, err
}

func readRequestRecordArtifactFile(path string) ([]*requestrecord.RequestRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	var records []*requestrecord.RequestRecord
	decoder := json.NewDecoder(reader)
	for {
		var envelope struct {
			RequestRecord *requestrecord.RequestRecord `json:"request_record"`
		}
		if err := decoder.Decode(&envelope); err != nil {
			if err == io.EOF {
				return records, nil
			}
			return nil, err
		}
		if envelope.RequestRecord != nil {
			records = append(records, envelope.RequestRecord)
		}
	}
}
