package protocoltest

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// upstreamOnlyModel is the model name the mock provider reports. The gateway
// must answer with the model the client asked for; the provider's own model
// id is routing internals and must never reach the client.
const upstreamOnlyModel = "upstream-only-model"

var upstreamModelField = regexp.MustCompile(`"model"\s*:\s*"[^"]*"`)

// withUpstreamModel rewrites every "model" field the scenario's mock provider
// emits to upstreamOnlyModel.
func withUpstreamModel(s Scenario) Scenario {
	rewrite := func(b []byte) []byte {
		return upstreamModelField.ReplaceAll(b, []byte(`"model":"`+upstreamOnlyModel+`"`))
	}
	builders := make(map[ResponseFormat]MockResponseBuilder, len(s.MockResponses))
	for format, b := range s.MockResponses {
		b := b
		if ns := b.NonStream; ns != nil {
			b.NonStream = func() (int, []byte) {
				status, body := ns()
				return status, rewrite(body)
			}
		}
		if st := b.Stream; st != nil {
			b.Stream = func() []string {
				lines := st()
				out := make([]string, len(lines))
				for i, l := range lines {
					out[i] = string(rewrite([]byte(l)))
				}
				return out
			}
		}
		builders[format] = b
	}
	s.MockResponses = builders
	return s
}

// TestResponseCarriesRequestedModel pins that every client response reports
// the requested (public) model, never the provider's model id.
func TestResponseCarriesRequestedModel(t *testing.T) {
	t.Parallel()

	for _, pair := range DefaultPairs() {
		for _, streaming := range []bool{false, true} {
			pair, streaming := pair, streaming
			t.Run(fmt.Sprintf("%s->%s/stream=%v", pair.Source, pair.Target, streaming), func(t *testing.T) {
				t.Parallel()

				env := NewTestEnv(t)
				scenario := withUpstreamModel(TextScenario())
				env.SetupRoute(pair.Source, pair.Target, scenario)
				model := env.findRouteModel(pair.Source, pair.Target, scenario.Name)
				path, body := buildRequest(pair.Source, model, streaming)

				status, raw := sendRaw(t, env, path, body)
				if status != http.StatusOK {
					t.Fatalf("status = %d: %s", status, raw)
				}
				if strings.Contains(raw, upstreamOnlyModel) {
					t.Fatalf("provider model id leaked to client:\n%s", raw)
				}
				if !strings.Contains(raw, model) {
					t.Fatalf("response does not report requested model %q:\n%s", model, raw)
				}
			})
		}
	}
}
