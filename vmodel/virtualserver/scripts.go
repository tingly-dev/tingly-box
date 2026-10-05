package virtualserver

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

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
// script answers /messages and /chat/completions.
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
	sig    string // modtime+size when last loaded
	id     string // model id registered from this file ("" if it failed)
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

	entries, _ := os.ReadDir(s.dir) // a missing dir is just "no scripts"
	current := map[string]string{}
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
			continue
		}
		current[name] = fmt.Sprintf("%d-%d", info.ModTime().UnixNano(), info.Size())
		names = append(names, name)
	}
	sort.Strings(names)

	sig := strings.Join(names, "|")
	for _, n := range names {
		sig += "|" + current[n]
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
			if f, ok := s.files[name]; ok && f.sig == current[name] && !f.failed {
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
func (s *scriptStore) load(name, sig string) bool {
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
		s.files[name] = scriptFile{sig: sig, failed: true}
		s.errors[name] = err.Error()
		logrus.Warnf("vmodel script %s ignored: %v", name, err)
		return false
	}
	if prev != cfg.ID {
		s.unregister(prev)
	}
	s.files[name] = scriptFile{sig: sig, id: cfg.ID}
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
	// Each registry gets its own model (and so its own cursor). Set replaces
	// in one locked step, so a concurrent request never finds the id missing.
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
