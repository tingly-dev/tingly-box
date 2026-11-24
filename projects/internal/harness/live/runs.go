package live

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// metaFile marks a directory as a harness run; only marked directories are
// ever opened or cleaned.
const metaFile = ".harness-run.json"

// RunMeta is what a run directory says about itself.
type RunMeta struct {
	Journey   string    `json:"journey"`
	Started   time.Time `json:"started"`
	ProjectID string    `json:"project_id,omitempty"`
}

// DefaultRoot is where runs go: /dev/shm when it exists (memory-backed, and
// its parents hold no CLAUDE.md or .claude that Claude Code would pick up),
// else the system temp directory.
func DefaultRoot() string {
	if info, err := os.Stat("/dev/shm"); err == nil && info.IsDir() {
		return filepath.Join("/dev/shm", "tingly-project-harness")
	}
	return filepath.Join(os.TempDir(), "tingly-project-harness")
}

func newRunDir(o Options, journey string) (string, error) {
	dir := o.WorkDir
	if dir == "" {
		root := o.Root
		if root == "" {
			root = DefaultRoot()
		}
		if err := os.MkdirAll(root, 0o755); err != nil {
			return "", err
		}
		d, err := os.MkdirTemp(root, journey+"-"+time.Now().Format("20060102-150405")+"-")
		if err != nil {
			return "", err
		}
		dir = d
	} else {
		if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
			return "", fmt.Errorf("work dir %s is not empty", dir)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return dir, writeMeta(dir, RunMeta{Journey: journey, Started: time.Now().UTC()})
}

func writeMeta(dir string, m RunMeta) error {
	data, err := json.MarshalIndent(m, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, metaFile), append(data, '\n'), 0o644)
}

// ReadRun reads the marker of a run directory.
func ReadRun(dir string) (RunMeta, error) {
	var m RunMeta
	data, err := os.ReadFile(filepath.Join(dir, metaFile))
	if err != nil {
		return m, fmt.Errorf("%s is not a harness run directory (no %s)", dir, metaFile)
	}
	return m, json.Unmarshal(data, &m)
}

// RunDir is one run directory under a root.
type RunDir struct {
	Dir  string
	Meta RunMeta
}

// Runs lists the run directories under root, newest first.
func Runs(root string) ([]RunDir, error) {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []RunDir
	for _, e := range entries {
		dir := filepath.Join(root, e.Name())
		if m, err := ReadRun(dir); err == nil && e.IsDir() {
			out = append(out, RunDir{Dir: dir, Meta: m})
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Meta.Started.After(out[b].Meta.Started) })
	return out, nil
}

// Clean removes run directories; anything without the run marker is left.
func Clean(dirs []string) error {
	for _, d := range dirs {
		if _, err := ReadRun(d); err != nil {
			return err
		}
		if err := os.RemoveAll(d); err != nil {
			return err
		}
	}
	return nil
}

// OpenOptions say how to reopen a run for looking at it.
type OpenOptions struct {
	Projects string
	// TB, when set, serves a placeholder model so the opened project still
	// answers; without it the project keeps its finished run's model URL.
	TB   string
	Port int
	Out  io.Writer
}

// Open serves a finished run's data with the same isolated environment until
// ctx ends, so its project can be looked at in the web UI.
func Open(ctx context.Context, dir string, o OpenOptions) error {
	m, err := ReadRun(dir)
	if err != nil {
		return err
	}
	w := worldAt(dir)
	if err := w.ensure(); err != nil {
		return err
	}
	stopProjects, url, err := w.startProjects(ctx, o.Projects, o.Port)
	if err != nil {
		return err
	}
	defer stopProjects()
	p := &projects{api: newAPI(url)}
	if err := p.waitHealthy(ctx); err != nil {
		return err
	}
	if o.TB != "" {
		if err := os.WriteFile(filepath.Join(dir, "placeholder.yaml"), []byte(placeholderModel), 0o644); err != nil {
			return err
		}
		stopModel, modelURL, err := w.startModelFile(ctx, o.TB, filepath.Join(dir, "placeholder.yaml"), "harness-placeholder", "")
		if err != nil {
			return err
		}
		defer stopModel()
		if _, err := p.do(ctx, http.MethodPut, "/api/v1/settings", connection(modelURL), nil); err != nil {
			return err
		}
	}
	page := url
	if m.ProjectID != "" {
		page = url + "/project/" + m.ProjectID
	}
	fmt.Fprintf(o.Out, "%s (%s) is open at %s\nIts data stays in %s; stop with Ctrl-C.\n", m.Journey, m.Started.Local().Format(time.DateTime), page, dir)
	if resp, err := http.Get(url + "/"); err == nil {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if strings.Contains(string(body), "web UI is not built") {
			fmt.Fprintf(o.Out, "Note: %s has no web UI built in; run `task projects:frontend`, rebuild it and open again.\n", o.Projects)
		}
	}
	<-ctx.Done()
	return nil
}

// placeholderModel answers whatever the opened project asks, so the UI can be
// used after the scripted run is over.
const placeholderModel = `id: harness-placeholder
kind: mock
mock:
  content: "(harness) The scripted run is over; this is a placeholder reply."
`
