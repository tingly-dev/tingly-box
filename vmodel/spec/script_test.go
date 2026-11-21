package spec

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const scriptDefinition = `
id: scene
kind: script
protocols: [anthropic]
script:
  count_tokens: 9
  tracks:
    main:
      bind: {system_contains: "agent"}
      steps:
        - id: hello
          after: ["signal:go"]
          reply: {content: "hello from the script"}
`

func post(t *testing.T, url, body string) (int, string) {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestScriptDefinitionServesEveryModelThroughFallback(t *testing.T) {
	defs, err := Decode([]byte(scriptDefinition))
	require.NoError(t, err)
	rt, err := New(defs)
	require.NoError(t, err)
	require.NoError(t, rt.SetFallback("scene"))
	assert.Equal(t, []string{"scene"}, rt.ScriptIDs())
	srv := httptest.NewServer(rt)
	defer srv.Close()

	status, body := post(t, srv.URL+"/v1/messages/count_tokens?beta=true", `{"model":"claude-sonnet-4-6","messages":[]}`)
	assert.Equal(t, 200, status)
	assert.JSONEq(t, `{"input_tokens":9}`, body)

	done := make(chan string, 1)
	go func() {
		_, b := post(t, srv.URL+"/v1/messages?beta=true", `{"model":"claude-sonnet-4-6","system":"an agent","tools":[{"name":"Bash"}],"messages":[{"role":"user","content":"hi"}]}`)
		done <- b
	}()
	status, body = post(t, srv.URL+"/_vmodel/scripts/scene/signals/go", "")
	assert.Equal(t, 204, status, body)
	resp, err := http.Get(srv.URL + "/_vmodel/scripts/scene/served?step=main.hello&timeout=5s")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, <-done, "hello from the script")

	status, body = post(t, srv.URL+"/_vmodel/scripts/scene/finish", "")
	require.Equal(t, 200, status)
	var rep struct {
		OK       bool `json:"ok"`
		Finished bool `json:"finished"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &rep))
	assert.True(t, rep.OK && rep.Finished, body)

	resp, err = http.Get(srv.URL + "/_vmodel/scripts/scene/served?step=main.nope&timeout=10ms")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 400, resp.StatusCode)
	resp, err = http.Get(srv.URL + "/_vmodel/scripts/other/report")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode)
}

func TestScriptDefinitionRules(t *testing.T) {
	for name, doc := range map[string]string{
		"openai protocol": "id: s\nkind: script\nprotocols: [openai_chat]\nscript: {tracks: {t: {bind: {}, steps: [{reply: {content: x}}]}}}",
		"expect block":    "id: s\nkind: script\nexpect: {http_status: 200}\nscript: {tracks: {t: {bind: {}, steps: [{reply: {content: x}}]}}}",
		"bad script":      "id: s\nkind: script\nscript: {tracks: {}}",
		"no block":        "id: s\nkind: script",
	} {
		_, err := Decode([]byte(doc))
		assert.Error(t, err, name)
	}
	defs, err := Decode([]byte(scriptDefinition))
	require.NoError(t, err)
	rt, err := New(defs)
	require.NoError(t, err)
	_, err = rt.Scenario("scene")
	assert.ErrorContains(t, err, "is a script")
}

func TestScriptDefinitionExportsAndLoadsAgain(t *testing.T) {
	defs, err := Decode([]byte(scriptDefinition))
	require.NoError(t, err)
	out, err := yaml.Marshal(defs)
	require.NoError(t, err)
	again, err := Decode(out)
	require.NoError(t, err, string(out))
	assert.Equal(t, "signal:go", again[0].Script.Tracks["main"].Steps[0].After[0])
	_, err = New(again)
	require.NoError(t, err)
}
