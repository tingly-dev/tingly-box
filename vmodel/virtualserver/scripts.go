package virtualserver

import (
	"errors"
	"fmt"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/vmodel"
	anthropicvm "github.com/tingly-dev/tingly-box/vmodel/anthropic"
	openaivm "github.com/tingly-dev/tingly-box/vmodel/openai"
)

// ScriptDirName is the directory (under the config dir) scanned for scripts.
const ScriptDirName = "vmodels"

// scriptStore keeps the registries in step with a directory of script files
// (see vmodel.ParseScript and .design/vmodel-script.md). Every script becomes
// one model, registered under its id in BOTH protocol registries so the same
// script answers /messages and /chat/completions, each protocol running its own
// independent copy of the program.
//
// Refresh is the only entry point and is cheap when nothing changed (a
// directory read plus a stat per file), so callers invoke it on every request
// instead of running a watcher: write a file, call the model, done. A file
// whose content changed is re-registered with a fresh cursor; a file that
// disappeared, or no longer parses, unregisters its model and the reason is
// kept in Problems so a miss can explain itself.
type scriptStore struct {
	dir  string
	anth *anthropicvm.Registry
	oai  *openaivm.Registry

	mu     sync.Mutex
	files  map[string]scriptFile // by file name
	errors map[string]string     // by file name; files that failed to load
	sig    string                // directory signature at the last scan
}

type scriptFile struct {
	stamp  fileStamp // identity of the file content when last loaded
	id     string    // model id registered from this file ("" if it failed)
	failed bool
}

func newScriptStore(dir string, a *anthropicvm.Registry, o *openaivm.Registry) *scriptStore {
	return &scriptStore{dir: dir, anth: a, oai: o, files: map[string]scriptFile{}, errors: map[string]string{}}
}

// Refresh reconciles the registries with the directory. Safe for concurrent
// use; concurrent callers serialise and the later ones find nothing to do.
func (s *scriptStore) Refresh() {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return // transient failure: keep what is loaded rather than drop every model
	}
	current := map[string]fileStamp{}
	var names []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		if ext := strings.ToLower(filepath.Ext(name)); ext != ".yaml" && ext != ".yml" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			// Briefly unstatable (an editor mid-rename): not "deleted". Keep
			// whatever we last loaded from it.
			if f, ok := s.files[name]; ok {
				current[name] = f.stamp
				names = append(names, name)
			}
			continue
		}
		current[name] = stampFile(filepath.Join(s.dir, name), info)
		names = append(names, name)
	}
	sort.Strings(names)

	sig := strings.Join(names, "|")
	for _, n := range names {
		sig += "|" + current[n].base + current[n].hash
	}
	if sig == s.sig {
		return
	}
	s.sig = sig

	// Removed files → unregister.
	for name, f := range s.files {
		if _, ok := current[name]; !ok {
			s.unregister(f.id)
			delete(s.files, name)
			delete(s.errors, name)
		}
	}
	// New or changed files → (re)load. A file that failed is retried on every
	// directory change, since the cause (an id held by a now-deleted file) may
	// be gone; a second pass lets two files swap ids.
	for pass := 0; pass < 2; pass++ {
		loaded := false
		for _, name := range names {
			if f, ok := s.files[name]; ok && f.stamp.same(current[name]) && !f.failed {
				continue
			}
			if pass == 1 && !s.files[name].failed {
				continue
			}
			loaded = s.load(name, current[name]) || loaded
		}
		if !loaded {
			break
		}
	}
}

// load (re)registers one file and reports whether it succeeded.
func (s *scriptStore) load(name string, stamp fileStamp) bool {
	prev := s.files[name].id
	data, err := os.ReadFile(filepath.Join(s.dir, name))
	var cfg vmodel.SequenceConfig
	if err == nil {
		cfg, err = vmodel.ParseScript(data, strings.TrimSuffix(name, filepath.Ext(name)))
	}
	if err == nil {
		err = s.register(name, &cfg)
	}
	if err != nil {
		// A file that does not load serves nothing: its old model goes too,
		// so what is served always matches what is on disk.
		s.unregister(prev)
		s.files[name] = scriptFile{stamp: stamp, failed: true}
		if s.errors[name] != err.Error() { // once per distinct problem, not on every directory change
			logrus.Warnf("vmodel script %s ignored: %v", name, err)
		}
		s.errors[name] = err.Error()
		return false
	}
	if prev != cfg.ID {
		s.unregister(prev)
	}
	s.files[name] = scriptFile{stamp: stamp, id: cfg.ID}
	delete(s.errors, name)
	logrus.Infof("vmodel script %s loaded as model %q", name, cfg.ID)
	return true
}

// register installs cfg in both registries, replacing this file's own earlier
// model of the same id but never a built-in model or another file's.
func (s *scriptStore) register(file string, cfg *vmodel.SequenceConfig) error {
	for other, f := range s.files {
		if other != file && !f.failed && f.id == cfg.ID {
			return fmt.Errorf("id %q is already used by %s", cfg.ID, other)
		}
	}
	if s.files[file].id != cfg.ID && (s.anth.Has(cfg.ID) || s.oai.Has(cfg.ID)) {
		return fmt.Errorf("id %q collides with a built-in model; pick another id", cfg.ID)
	}
	// Each registry gets its own model and so its own cursor: a request on one
	// wire must never consume a step of the other. Set replaces in one locked
	// step, so a concurrent request never finds the id missing.
	s.anth.Set(anthropicvm.NewSequenceModel(cfg))
	s.oai.Set(openaivm.NewSequenceModel(cfg))
	return nil
}

func (s *scriptStore) unregister(id string) {
	if id == "" {
		return
	}
	s.anth.Unregister(id)
	s.oai.Unregister(id)
}

// fileStamp identifies a file's content cheaply: modtime and size (base), plus
// a content hash while the file is fresh. File systems stamp mtime coarsely
// (milliseconds), so a same-size rewrite right after the last one can carry an
// identical mtime; hashing files modified in the last couple of seconds closes
// that window, and ageing out of it must not look like a change.
type fileStamp struct {
	base string
	hash string // "" once the file is old enough not to need it
}

// same reports whether two stamps describe the same content: equal base, and
// equal hash whenever both have one.
func (a fileStamp) same(b fileStamp) bool {
	return a.base == b.base && (a.hash == "" || b.hash == "" || a.hash == b.hash)
}

func stampFile(path string, info fs.FileInfo) fileStamp {
	st := fileStamp{base: fmt.Sprintf("%d-%d", info.ModTime().UnixNano(), info.Size())}
	if time.Since(info.ModTime()) < 2*time.Second {
		if data, err := os.ReadFile(path); err == nil {
			h := fnv.New64a()
			h.Write(data)
			st.hash = fmt.Sprintf("-%x", h.Sum64())
		}
	}
	return st
}

// Problems returns the current load errors, "file: reason", sorted. Used to
// make a model-not-found response explain a script that failed to load.
func (s *scriptStore) Problems() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.errors))
	for name, msg := range s.errors {
		out = append(out, name+": "+msg)
	}
	sort.Strings(out)
	return out
}
