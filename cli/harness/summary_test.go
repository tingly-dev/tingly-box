package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocoltest"
)

// writeSummary writes a CSV with the given header + rows to a temp file and
// returns its path. The header is the canonical summaryCSVColumns when empty.
func writeSummary(t *testing.T, header string, rows ...string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "harness-summary.csv")
	content := ""
	if header != "" {
		content += header + "\n"
	}
	for _, r := range rows {
		content += r + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write summary: %v", err)
	}
	return path
}

func TestLoadFailedKeys(t *testing.T) {
	header := "timestamp,agent,entry,model,api_style,request_model,provider_baseurl,status,duration_ms,exit_code,output_id,prompt_summary,error"
	cases := []struct {
		name string
		rows []string
		want []resumeKey // sorted for stable compare
	}{
		{
			name: "picks only FAIL and TIMEOUT, latest row wins",
			rows: []string{
				// claude/acme failed then passed -> NOT red (latest = PASS)
				"t1,claude,acme,m,s,rm,u,FAIL,100,1,o,p,err",
				"t2,claude,acme,m,s,rm,u,PASS,100,0,o,p,",
				// claude/beta passed then timed out -> red (latest = TIMEOUT)
				"t1,claude,beta,m,s,rm,u,PASS,100,0,o,p,",
				"t2,claude,beta,m,s,rm,u,TIMEOUT,120000,-1,o,p,timeout",
				// codex/gamma fail -> red
				"t1,codex,gamma,m,s,rm,u,FAIL,100,1,o,p,err",
				// opencode/delta pass -> not red
				"t1,opencode,delta,m,s,rm,u,PASS,100,0,o,p,",
			},
			want: []resumeKey{
				{Agent: "claude", Entry: "beta"},
				{Agent: "codex", Entry: "gamma"},
			},
		},
		{
			name: "all green -> empty set",
			rows: []string{
				"t1,claude,acme,m,s,rm,u,PASS,100,0,o,p,",
				"t1,codex,gamma,m,s,rm,u,PASS,100,0,o,p,",
			},
			want: nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := writeSummary(t, header, c.rows...)
			got, err := loadFailedKeys(path)
			if err != nil {
				t.Fatalf("loadFailedKeys: %v", err)
			}
			var gotKeys []resumeKey
			for k := range got {
				gotKeys = append(gotKeys, k)
			}
			sort.Slice(gotKeys, func(i, j int) bool {
				if gotKeys[i].Agent != gotKeys[j].Agent {
					return gotKeys[i].Agent < gotKeys[j].Agent
				}
				return gotKeys[i].Entry < gotKeys[j].Entry
			})
			want := c.want
			sort.Slice(want, func(i, j int) bool {
				if want[i].Agent != want[j].Agent {
					return want[i].Agent < want[j].Agent
				}
				return want[i].Entry < want[j].Entry
			})
			if !reflect.DeepEqual(gotKeys, want) {
				t.Fatalf("got %v, want %v", gotKeys, want)
			}
		})
	}
}

func TestLoadFailedKeysMissingFile(t *testing.T) {
	got, err := loadFailedKeys(filepath.Join(t.TempDir(), "nope.csv"))
	if !errors.Is(err, errNoPriorSummary) {
		t.Fatalf("want errNoPriorSummary, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty set, got %v", got)
	}
}

func TestOnlyFailingFlagParses(t *testing.T) {
	cli, parser := newTestParser(t)
	if _, err := parser.Parse([]string{"agent", "claude", "--config", "x.yaml", "--only-failing"}); err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if !cli.Agent.OnlyFailing {
		t.Error("OnlyFailing not set")
	}
}

// An unexpanded env ref (${VAR}/$VAR whose var was unset) must be treated as a
// missing apikey so the entry is skipped, not sent upstream with a literal.
func TestMissingFieldsFlagsUnexpandedEnvRef(t *testing.T) {
	base := protocoltest.RealModelEntry{
		BaseURL: "https://api.example.com", Model: "m", APIStyle: "openai",
	}
	cases := []struct {
		name string
		key  string
		want bool // true = apikey flagged missing
	}{
		{"braced unset", "${DEFINITELY_UNSET}", true},
		{"bare unset", "$DEFINITELY_UNSET", true},
		{"empty", "", true},
		{"placeholder", "YOUR_API_KEY", true},
		{"real key", "sk-real", false},
		{"key with embedded ref value", "sk-${SOME_VAR}", false}, // partial — not a pure ref
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := base
			e.APIKey = c.key
			miss := missingFields(e)
			got := false
			for _, f := range miss {
				if f == "apikey" {
					got = true
				}
			}
			if got != c.want {
				t.Fatalf("apikey-missing for %q: got %v want %v (miss=%v)", c.key, got, c.want, miss)
			}
		})
	}
}
