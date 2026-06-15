package benchmark

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/protocol/sse"
	"github.com/tingly-dev/tingly-box/vmodel"
	"github.com/tingly-dev/tingly-box/vmodel/benchmark/scenario"
)

// readBody reads the full request body. The capture middleware has already
// restored r.Body to a re-readable reader, so this handler (terminal) consumes
// it without needing to restore.
func readBody(r *http.Request) []byte {
	b, _ := io.ReadAll(r.Body)
	return b
}

// scenarioResponder serves scenario.MockResponseBuilder fixtures over the four
// provider-native routes. It is the reusable home of the scenario-serving logic
// that protocoltest.VirtualServer implements inline today; the parent Server
// wraps it with the shared capture middleware.
type scenarioResponder struct {
	scenarios *vmodel.GenericRegistry[scenario.Scenario]
	mux       *http.ServeMux
}

func newScenarioResponder(reg *vmodel.GenericRegistry[scenario.Scenario]) http.Handler {
	sr := &scenarioResponder{scenarios: reg, mux: http.NewServeMux()}
	sr.mux.HandleFunc("/v1/chat/completions", sr.handle(scenario.FormatOpenAIChat))
	sr.mux.HandleFunc("/v1/responses", sr.handleResponses)
	sr.mux.HandleFunc("/v1/messages", sr.handle(scenario.FormatAnthropic))
	sr.mux.HandleFunc("/v1beta/models/", sr.handleGoogle)
	return sr
}

func (sr *scenarioResponder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sr.mux.ServeHTTP(w, r)
}

// handle returns an HTTP handler that serves the given format for the scenario
// detected from the request body's model field.
func (sr *scenarioResponder) handle(format scenario.ResponseFormat) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body := readBody(r)
		streaming := parseStreamFlag(body)
		sc := sr.detect(body)

		builder, ok := sc.MockResponses[format]
		if !ok {
			http.Error(w, "no "+string(format)+" mock response for scenario "+sc.Name, http.StatusInternalServerError)
			return
		}
		writeBuilderResponse(w, builder, streaming)
	}
}

// handleResponses serves the OpenAI Responses format, falling back to OpenAI
// Chat when a scenario does not define a Responses-specific builder.
func (sr *scenarioResponder) handleResponses(w http.ResponseWriter, r *http.Request) {
	body := readBody(r)
	streaming := parseStreamFlag(body)
	sc := sr.detect(body)

	builder, ok := sc.MockResponses[scenario.FormatOpenAIResponses]
	if !ok {
		builder, ok = sc.MockResponses[scenario.FormatOpenAIChat]
		if !ok {
			http.Error(w, "no openai_responses or openai_chat mock response for scenario "+sc.Name, http.StatusInternalServerError)
			return
		}
	}
	writeBuilderResponse(w, builder, streaming)
}

// handleGoogle serves the Google format. Google encodes the model in the URL
// (/v1beta/models/{model}:generateContent) and streams via streamGenerateContent.
func (sr *scenarioResponder) handleGoogle(w http.ResponseWriter, r *http.Request) {
	body := readBody(r)
	streaming := strings.Contains(r.URL.Path, "streamGenerateContent")
	sc := sr.detectFromURLOrBody(r.URL.Path, body)

	builder, ok := sc.MockResponses[scenario.FormatGoogle]
	if !ok {
		http.Error(w, "no google mock response for scenario "+sc.Name, http.StatusInternalServerError)
		return
	}
	writeBuilderResponse(w, builder, streaming)
}

// writeBuilderResponse serves a MockResponseBuilder. For streaming requests it
// writes a 200 SSE stream, except when the builder declares a pre-content HTTP
// error (StreamHTTPError >= 400) — those fail at the status line, as a real
// provider rejects an auth/rate-limit/5xx error before any SSE frame.
func writeBuilderResponse(w http.ResponseWriter, builder scenario.MockResponseBuilder, streaming bool) {
	if streaming && builder.StreamHTTPError < 400 && builder.Stream != nil {
		sse.WriteSSEResponse(w, builder.Stream())
		return
	}
	if builder.NonStream == nil {
		http.Error(w, "scenario builder has no non-stream response", http.StatusInternalServerError)
		return
	}
	status, body := builder.NonStream()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// detect resolves the scenario for a request body. It accepts both the
// "virtual-model-{scenario}" model form (as protocoltest sets) and a raw
// scenario name as the model, falling back to the first registered scenario.
func (sr *scenarioResponder) detect(body []byte) scenario.Scenario {
	if name := scenarioNameFromBody(body); name != "" && sr.scenarios.Has(name) {
		return sr.scenarios.Get(name)
	}
	return sr.first()
}

func (sr *scenarioResponder) detectFromURLOrBody(urlPath string, body []byte) scenario.Scenario {
	const prefix = "virtual-model-"
	for _, part := range strings.Split(urlPath, "/") {
		if strings.HasPrefix(part, prefix) {
			remaining := part[len(prefix):]
			name := remaining
			if i := strings.LastIndex(remaining, "-"); i > 0 {
				name = remaining[i+1:]
			}
			if i := strings.IndexByte(name, ':'); i >= 0 {
				name = name[:i]
			}
			if sr.scenarios.Has(name) {
				return sr.scenarios.Get(name)
			}
		}
	}
	return sr.detect(body)
}

func (sr *scenarioResponder) first() scenario.Scenario {
	for _, s := range sr.scenarios.List() {
		return s
	}
	return scenario.Scenario{}
}

// scenarioNameFromBody extracts a scenario name from the request body's model
// field, honoring the "virtual-model-{scenario}" convention or a raw name.
func scenarioNameFromBody(body []byte) string {
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return ""
	}
	model, _ := m["model"].(string)
	if model == "" {
		return ""
	}
	const prefix = "virtual-model-"
	if strings.HasPrefix(model, prefix) {
		return model[len(prefix):]
	}
	return model
}

func parseStreamFlag(body []byte) bool {
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return false
	}
	flag, _ := m["stream"].(bool)
	return flag
}
