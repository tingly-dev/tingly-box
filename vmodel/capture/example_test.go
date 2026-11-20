package capture

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The checked-in example (vmodel/examples/capture) is evidence of what a
// capture looks like. These tests keep it honest: its index must still locate
// real requests in the raw files, it must show what only a raw capture can
// (header-name casing and order as sent), and it must stay redacted.

const exampleDir = "../examples/capture/claude-code-2.1.329"

type exampleEntry struct {
	Seq    int64    `json:"seq"`
	Conn   int64    `json:"conn"`
	In     [2]int64 `json:"in"`
	Out    [2]int64 `json:"out"`
	Method string   `json:"method"`
	Target string   `json:"target"`
	Status int      `json:"status"`
}

// exampleRequest loads the single request of one example cell.
func exampleRequest(t *testing.T, cell string) (head, body string) {
	t.Helper()
	dir := filepath.Join(exampleDir, cell)
	indexData, err := os.ReadFile(filepath.Join(dir, "index.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(indexData)), "\n")
	if len(lines) != 1 {
		t.Fatalf("%s: %d requests in the index, want exactly 1 (a retry would show up as a second one)", cell, len(lines))
	}
	var e exampleEntry
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
		t.Fatal(err)
	}
	in, err := os.ReadFile(filepath.Join(dir, "raw", "conn-0001.in"))
	if err != nil {
		t.Fatal(err)
	}
	if e.In[0] != 0 || e.In[1] != int64(len(in)) {
		t.Fatalf("%s: in range %v does not cover the %d-byte raw file", cell, e.In, len(in))
	}
	head, body, ok := strings.Cut(string(in[e.In[0]:e.In[1]]), "\r\n\r\n")
	if !ok {
		t.Fatalf("%s: no CRLF CRLF between head and body (line endings normalised?)", cell)
	}
	if !strings.HasPrefix(head, e.Method+" "+e.Target+" HTTP/1.1\r\n") || e.Seq != 1 || e.Conn != 1 || e.Status != 200 {
		t.Fatalf("%s: index %+v disagrees with request line %q", cell, e, strings.SplitN(head, "\r\n", 2)[0])
	}
	out, err := os.ReadFile(filepath.Join(dir, "raw", "conn-0001.out"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out[e.Out[0]:e.Out[1]], []byte("HTTP/1.1 200 OK\r\n")) {
		t.Fatalf("%s: out range does not start with the response status line", cell)
	}
	return head, body
}

func TestExampleIndexLocatesRealRequests(t *testing.T) {
	for _, cell := range []string{"oauth-proxy", "oauth-direct"} {
		t.Run(cell, func(t *testing.T) {
			head, body := exampleRequest(t, cell)
			m := regexp.MustCompile(`(?i)\r\ncontent-length: (\d+)`).FindStringSubmatch(head)
			if m == nil {
				t.Fatal("no Content-Length header")
			}
			if n, _ := strconv.Atoi(m[1]); n != len(body) {
				t.Fatalf("Content-Length %d, body %d bytes", n, len(body))
			}
			var doc map[string]json.RawMessage
			if err := json.Unmarshal([]byte(body), &doc); err != nil {
				t.Fatalf("body is not a JSON object: %v", err)
			}
			for _, k := range []string{"model", "messages", "system", "tools", "metadata", "stream"} {
				if _, ok := doc[k]; !ok {
					t.Errorf("body lacks %q", k)
				}
			}
		})
	}
}

// What net/http would have lost: names as the client wrote them, in the order
// it wrote them.
func TestExampleKeepsHeaderCasingAndOrder(t *testing.T) {
	head, _ := exampleRequest(t, "oauth-proxy")
	order := []string{"\r\nAuthorization: ", "\r\nUser-Agent: ", "\r\nX-Stainless-Arch: ", "\r\nX-Stainless-Timeout: ",
		"\r\nanthropic-beta: ", "\r\nanthropic-version: ", "\r\nx-app: ", "\r\nConnection: ", "\r\nHost: "}
	last := -1
	for _, h := range order {
		i := strings.Index(head, h)
		if i < 0 {
			t.Fatalf("header %q (exact casing) not in the capture", strings.TrimSpace(h))
		}
		if i < last {
			t.Fatalf("header %q is out of order", strings.TrimSpace(h))
		}
		last = i
	}
}

// The proxy and direct cells differ the way the research notes say they do.
func TestExampleProxyVersusDirect(t *testing.T) {
	proxyHead, proxyBody := exampleRequest(t, "oauth-proxy")
	directHead, directBody := exampleRequest(t, "oauth-direct")
	for _, h := range []string{"x-claude-code-prompt-id", "x-client-request-id", "x-claude-1p-request-class"} {
		if strings.Contains(proxyHead, "\r\n"+h+": ") || !strings.Contains(directHead, "\r\n"+h+": ") {
			t.Errorf("%s should be a direct-only header", h)
		}
	}
	if strings.Contains(proxyBody, "cch=") || !strings.Contains(directBody, "cch=xxxxx;") {
		t.Error("cch should appear in the direct billing header only (redacted to xxxxx)")
	}
	if strings.Contains(proxyHead, "prompt-caching-scope") || !strings.Contains(directHead, "prompt-caching-scope") {
		t.Error("prompt-caching-scope should be a direct-only beta")
	}
}

var (
	hex64       = regexp.MustCompile(`[0-9a-fA-F]{64}`)
	anyUUID     = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	placeholder = regexp.MustCompile(`^00000000-0000-4000-8000-\d{12}$`)
	secretToken = regexp.MustCompile(`sk-ant-[a-z]+\d*-?(\S{0,12})`)
)

// longStrings returns every JSON string value longer than n characters.
func longStrings(v any, n int) []string {
	var out []string
	switch v := v.(type) {
	case string:
		if len([]rune(v)) > n {
			out = append(out, v[:40])
		}
	case []any:
		for _, e := range v {
			out = append(out, longStrings(e, n)...)
		}
	case map[string]any:
		for _, e := range v {
			out = append(out, longStrings(e, n)...)
		}
	}
	return out
}

// The example must stay shareable: if it is re-captured, run redact.py first.
func TestExampleIsRedacted(t *testing.T) {
	err := filepath.Walk(filepath.Dir(exampleDir), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || strings.HasSuffix(path, ".py") || strings.HasSuffix(path, ".md") || strings.HasSuffix(path, ".gitattributes") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		for _, bad := range []string{"/home/", "/Users/", "/root/", "/dev/shm", "/tmp/", "CLAUDE.md", "x0079"} {
			if strings.Contains(text, bad) {
				t.Errorf("%s contains %q", path, bad)
			}
		}
		for _, h := range hex64.FindAllString(text, -1) {
			if strings.Trim(h, "0") != "" {
				t.Errorf("%s contains a 64-hex id %s…", path, h[:8])
			}
		}
		if !strings.HasSuffix(path, "index.jsonl") && !strings.HasSuffix(path, "meta.json") && !strings.Contains(path, ".out") {
			for _, u := range anyUUID.FindAllString(text, -1) {
				if !placeholder.MatchString(u) {
					t.Errorf("%s contains a real-looking uuid %s", path, u)
				}
			}
			for _, m := range secretToken.FindAllStringSubmatch(text, -1) {
				if !strings.HasPrefix(m[1], "[redacted]") {
					t.Errorf("%s contains a credential: %q", path, m[0])
				}
			}
			if strings.Contains(path, filepath.Join("raw", "conn-0001.in")) {
				_, body, _ := strings.Cut(text, "\r\n\r\n")
				var doc any
				if err := json.Unmarshal([]byte(body), &doc); err != nil {
					t.Errorf("%s: body not JSON: %v", path, err)
				}
				if long := longStrings(doc, 200); len(long) > 0 {
					t.Errorf("%s still has long prompt/tool text: %q", path, long)
				}
			}
		}
		if strings.HasSuffix(path, "meta.json") && !strings.Contains(text, `"redacted": true`) {
			t.Errorf("%s is not marked redacted", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
