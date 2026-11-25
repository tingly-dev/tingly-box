package obs

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/sirupsen/logrus"
)

// RetentionPolicy bounds how much response/request recording stays on disk.
// The default gzip exporter prunes lazily as it appends.
//
// Cleanup is deliberately conservative: it only removes session log files
// (.jsonl.gz / .jsonl) under the recording root, grouped by the YYYY-MM-DD
// session date directory. Content-addressed blobs (opt-in CAS exporter) are
// shared between records and are left untouched.
type RetentionPolicy struct {
	// MaxAge deletes session files older than this. <= 0 disables age pruning.
	MaxAge time.Duration
	// MaxBytes caps the total size of session log files; the oldest are
	// deleted first when the budget is exceeded. <= 0 disables size pruning.
	MaxBytes int64
	// PruneInterval throttles how often the exporter walks the tree. <= 0
	// prunes at most once per export batch.
	PruneInterval time.Duration
}

// DefaultRetention is the policy applied when none is configured. Seven days
// is long enough to reconstruct a recent incident (the usual debugging window)
// without letting response bodies — which may contain user content — pile up
// indefinitely; 1 GiB bounds disk use for a single-tenant gateway.
func DefaultRetention() RetentionPolicy {
	return RetentionPolicy{
		MaxAge:        7 * 24 * time.Hour,
		MaxBytes:      1 << 30, // 1 GiB
		PruneInterval: time.Hour,
	}
}

// normalize fills zero fields with defaults (keeping explicitly negative
// values as "disabled").
func (p RetentionPolicy) normalize() RetentionPolicy {
	d := DefaultRetention()
	if p.MaxAge == 0 {
		p.MaxAge = d.MaxAge
	}
	if p.MaxBytes == 0 {
		p.MaxBytes = d.MaxBytes
	}
	if p.PruneInterval == 0 {
		p.PruneInterval = d.PruneInterval
	}
	return p
}

// PruneRecordDir deletes session log files under baseDir according to policy
// and returns how many files it removed. It is safe to call on a missing
// directory (no-op). Errors on individual files are logged, not returned, so
// one unreadable file cannot wedge recording.
func PruneRecordDir(baseDir string, policy RetentionPolicy) (int, error) {
	if baseDir == "" {
		return 0, nil
	}
	policy = policy.normalize()
	removed := 0
	now := time.Now()

	type fileEntry struct {
		path    string
		modTime time.Time
		size    int64
	}
	var files []fileEntry

	err := filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			logrus.Debugf("obs: retention walk %s: %v", path, err)
			return nil
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !isRecordLogFile(name) {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		// Retention follows on-disk age (mtime), not the record's logical
		// timestamp: a session file that is still being appended to stays
		// young, and historical/replayed data is not deleted just because its
		// payload timestamp is old.
		age := info.ModTime()
		if policy.MaxAge > 0 && now.Sub(age) > policy.MaxAge {
			if rmErr := os.Remove(path); rmErr != nil {
				logrus.Debugf("obs: retention remove %s: %v", path, rmErr)
			} else {
				removed++
			}
			return nil
		}
		files = append(files, fileEntry{path: path, modTime: info.ModTime(), size: info.Size()})
		return nil
	})
	if err != nil {
		return removed, err
	}

	if policy.MaxBytes > 0 {
		var total int64
		for _, f := range files {
			total += f.size
		}
		if total > policy.MaxBytes {
			sort.Slice(files, func(i, j int) bool { return files[i].modTime.Before(files[j].modTime) })
			for _, f := range files {
				if total <= policy.MaxBytes {
					break
				}
				if rmErr := os.Remove(f.path); rmErr != nil {
					logrus.Debugf("obs: retention remove %s: %v", f.path, rmErr)
					continue
				}
				total -= f.size
				removed++
			}
		}
	}

	removeEmptyDateDirs(baseDir)
	return removed, nil
}

func isRecordLogFile(name string) bool {
	if len(name) == 0 {
		return false
	}
	if filepath.Ext(name) == ".jsonl" {
		return true
	}
	// .jsonl.gz: filepath.Ext is ".gz", so check the full suffix.
	const gzSuffix = ".jsonl.gz"
	return len(name) > len(gzSuffix) && name[len(name)-len(gzSuffix):] == gzSuffix
}

// removeEmptyDateDirs removes session date directories that retention emptied,
// walking up to (but never deleting) the recording root.
func removeEmptyDateDirs(baseDir string) {
	var dirs []string
	_ = filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if _, ok := parseDateName(d.Name()); ok {
			dirs = append(dirs, path)
		}
		return nil
	})
	// Deepest first so nested empties collapse.
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			continue
		}
		_ = os.Remove(dir)
	}
}

func parseDateName(name string) (time.Time, bool) {
	day, err := time.Parse("2006-01-02", name)
	return day, err == nil
}
