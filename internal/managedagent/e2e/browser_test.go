package e2e_test

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/tingly-dev/tingly-box/internal"
)

// Journey: everything the user actually touches. The built web UI (embedded
// in the server binary, as shipped) is driven with Playwright against a real
// server, real CLI and a scripted model: pick a folder, start a task, read
// the answer, steer, approve a command, see the folder on the Repositories
// page. Screenshots of every step land in TB_E2E_OUT (default: a temp dir
// printed on failure).
//
// Prerequisites beyond the API journeys, each reported as a skip when missing:
// the frontend built into internal/web/dist (`task web:dist`) BEFORE `go test`
// compiles this package, node with the frontend's node_modules installed, and
// a Chromium (TB_E2E_CHROME or Playwright's own).
func TestJourney_Browser(t *testing.T) {
	requireE2E(t)
	if f, err := internal.WebDistAssets.Open("web/dist/index.html"); err != nil {
		t.Skip("web UI not embedded: run `task web:dist` (or `cd frontend && pnpm build && cp -R dist/* ../internal/web/dist/`) and re-run")
	} else {
		f.Close()
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	_, here, _, _ := runtime.Caller(0)
	repo := filepath.Clean(filepath.Join(filepath.Dir(here), "..", "..", ".."))
	frontend := filepath.Join(repo, "frontend")
	if _, err := os.Stat(filepath.Join(frontend, "node_modules", "playwright")); err != nil {
		t.Skip("frontend/node_modules/playwright missing: run `pnpm install --frozen-lockfile` in frontend/")
	}
	chrome := os.Getenv("TB_E2E_CHROME")
	if chrome == "" {
		if _, err := os.Stat("/tmp/chrome/chrome-linux64/chrome"); err == nil {
			chrome = "/tmp/chrome/chrome-linux64/chrome"
		}
	}

	up := newScriptedUpstream(t,
		upstreamTurn{Text: "Hello from the browser journey"},
		upstreamTurn{Bash: "touch browser-marker.txt && echo browser-marker"},
		upstreamTurn{Text: "ran it (browser)"},
	)
	s := bootStack(t, up)
	// The Tasks rail is an experimental feature; switch it on as the UI would.
	if code := s.do(http.MethodPut, "/api/v1/scenario/_global/flag/managed_agent", map[string]any{"value": true}, nil); code != 200 {
		t.Fatalf("enable managed_agent flag: %d", code)
	}
	dir := newGitDir(t, "browser-playground")

	out := os.Getenv("TB_E2E_OUT")
	if out == "" {
		out = filepath.Join(t.TempDir(), "shots")
	}
	cmd := exec.Command(node, filepath.Join(filepath.Dir(here), "browser", "managed_agent.mjs"))
	cmd.Dir = frontend
	cmd.Env = append(os.Environ(),
		"TB_BASE_URL="+s.base, "TB_TOKEN="+s.token, "TB_FOLDER="+dir, "TB_OUT="+out,
		"TB_FRONTEND_DIR="+frontend, "TB_CHROME="+chrome,
	)
	res, err := cmd.CombinedOutput()
	t.Logf("browser journey output:\n%s", res)
	if err != nil {
		t.Fatalf("browser journey failed (%v); screenshots in %s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "browser-marker.txt")); err != nil {
		t.Fatalf("the approved command did not run in %s", dir)
	}
	t.Logf("screenshots in %s", out)
}
