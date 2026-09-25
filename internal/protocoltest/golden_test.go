package protocoltest

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Golden wire snapshots pin the exact bytes the gateway sends upstream and
// returns to the client, so a refactor that is meant to be behavior-neutral
// can prove it. Regenerate after an intended change with:
//
//	go test ./internal/protocoltest -run TestGoldenWire -update
//
// and review the diff like any other code change.
var updateGolden = flag.Bool("update", false, "rewrite golden files under testdata/golden")

// Values that legitimately differ run to run. IDs are not blanked but
// numbered by first appearance, so a response that must echo a request id
// (tool_use_id, call_id, ...) still has to point at the same placeholder.
var (
	goldenIDValue = regexp.MustCompile(`"(id|item_id|call_id|tool_use_id|response_id|previous_response_id|message_id)"\s*:\s*"([^"]*)"`)
	goldenTimeVal = regexp.MustCompile(`"(created|created_at|completed_at)"\s*:\s*[0-9]+`)
	// Random by design: OpenAI stream obfuscation padding.
	goldenRandomVal = regexp.MustCompile(`"(obfuscation)"\s*:\s*"[^"]*"`)
)

func normalizeGolden(s string) string {
	ids := map[string]string{}
	s = goldenIDValue.ReplaceAllStringFunc(s, func(m string) string {
		sub := goldenIDValue.FindStringSubmatch(m)
		key, val := sub[1], sub[2]
		if val == "" {
			return m
		}
		ph, ok := ids[val]
		if !ok {
			ph = fmt.Sprintf("<ID%d>", len(ids)+1)
			ids[val] = ph
		}
		return fmt.Sprintf(`"%s":"%s"`, key, ph)
	})
	s = goldenRandomVal.ReplaceAllString(s, `"$1":"<RANDOM>"`)
	return goldenTimeVal.ReplaceAllString(s, `"$1":<TS>`)
}

// goldenJSON re-encodes a JSON body with sorted keys and indentation so
// golden diffs show field-level changes; non-JSON bodies are returned as-is.
func goldenJSON(raw []byte) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(out)
}

func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run with -update to create): %v", path, err)
	}
	if !bytes.Equal(want, []byte(got)) {
		t.Errorf("wire output differs from %s (rerun with -update if intended):\n%s", path, lineDiff(string(want), got))
	}
}

// lineDiff is a minimal first-divergence report; the golden file itself is
// the full reference.
func lineDiff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(w) || i < len(g); i++ {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl != gl {
			lo := max(0, i-3)
			return fmt.Sprintf("first difference at line %d\n--- want\n%s\n+++ got\n%s",
				i+1, strings.Join(w[lo:min(len(w), i+4)], "\n"), strings.Join(g[lo:min(len(g), i+4)], "\n"))
		}
	}
	return "(no line difference; trailing bytes differ)"
}

// TestGoldenWire snapshots, per protocol pair, every scenario in both
// streaming modes: the request the provider received and the raw response
// the client received.
func TestGoldenWire(t *testing.T) {
	t.Parallel()

	scenarios := AllScenarios()
	sort.Slice(scenarios, func(i, j int) bool { return scenarios[i].Name < scenarios[j].Name })

	for _, pair := range DefaultPairs() {
		pair := pair
		t.Run(fmt.Sprintf("%s->%s", pair.Source, pair.Target), func(t *testing.T) {
			t.Parallel()

			var doc strings.Builder
			for _, scenario := range scenarios {
				for _, streaming := range []bool{false, true} {
					env := NewTestEnv(t)
					env.SetupRoute(pair.Source, pair.Target, scenario)
					model := env.findRouteModel(pair.Source, pair.Target, scenario.Name)
					path, body := buildRequest(pair.Source, model, streaming)
					status, raw := sendRaw(t, env, path, body)

					upstream := "(no upstream request)"
					if req := env.virtual.LastRequest(cacheControlEndpoint(pair.Target)); req != nil {
						upstream = req.Method + " " + req.Path + "\n" + goldenJSON(req.Body)
					}
					// IDs are numbered per case: some are time-based, so two cases
					// in the same second can share one.
					doc.WriteString(normalizeGolden(fmt.Sprintf("=== %s stream=%v\n--- upstream request\n%s\n--- client response (status %d)\n%s\n\n",
						scenario.Name, streaming, upstream, status, strings.TrimRight(raw, "\n"))))
					env.Close()
				}
			}
			assertGolden(t, fmt.Sprintf("wire/%s__%s.txt", pair.Source, pair.Target), doc.String())
		})
	}
}
