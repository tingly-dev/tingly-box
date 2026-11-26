package spec

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tingly-dev/tingly-box/vmodel/benchmark/check"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/vmodel"
	"github.com/tingly-dev/tingly-box/vmodel/anthropic"
	"github.com/tingly-dev/tingly-box/vmodel/benchmark"
	"github.com/tingly-dev/tingly-box/vmodel/benchmark/scenario"
	"github.com/tingly-dev/tingly-box/vmodel/decision"
	"github.com/tingly-dev/tingly-box/vmodel/openai"
	"github.com/tingly-dev/tingly-box/vmodel/script"
	"github.com/tingly-dev/tingly-box/vmodel/virtualserver"
)

// Catalog names Go behavior factories explicitly. Factories must create fresh
// instances so sequence cursors and dynamic callbacks never leak across runs.
type Factory func(Definition) (scenario.Scenario, error)
type Models struct {
	Anthropic anthropic.VirtualModel
	OpenAI    openai.VirtualModel
	Decision  decision.VirtualModel
}
type ModelFactory func(Definition) (Models, error)
type Catalog struct {
	factories map[string]Factory
	models    map[string]ModelFactory
}

func NewCatalog() *Catalog {
	c := &Catalog{factories: map[string]Factory{}, models: map[string]ModelFactory{}}
	for _, s := range scenario.AllScenarios() {
		name := s.Name
		c.factories["scenario/"+name] = func(d Definition) (scenario.Scenario, error) {
			for _, s := range scenario.AllScenarios() {
				if s.Name == name {
					s.Name = d.ID
					return s, nil
				}
			}
			return scenario.Scenario{}, fmt.Errorf("missing scenario %s", name)
		}
	}
	return c
}
func (c *Catalog) Register(name string, f Factory) error {
	if name == "" || f == nil {
		return fmt.Errorf("behavior name and factory are required")
	}
	if _, ok := c.factories[name]; ok || c.models[name] != nil {
		return fmt.Errorf("behavior %q already registered", name)
	}
	c.factories[name] = f
	return nil
}

// RegisterModels adapts custom Go virtual models, choosers and transform chains.
// The stable name is serializable; the implementation remains Go code.
func (c *Catalog) RegisterModels(name string, f ModelFactory) error {
	if name == "" || f == nil {
		return fmt.Errorf("behavior name and factory are required")
	}
	if c.models[name] != nil || c.factories[name] != nil {
		return fmt.Errorf("behavior %q already registered", name)
	}
	c.models[name] = f
	return nil
}

// Runtime is the common service used by the CLI and by scenario adapters.
type Runtime struct {
	service     *virtualserver.Service
	semantic    http.Handler
	definitions map[string]Definition
	wire        map[string]scenario.Scenario
	protocols   map[string]map[scenario.ResponseFormat]bool
	scripts     map[string]*script.Engine
	fallback    string
}

// SetFallback makes request model ids that no definition names be served by
// the definition fallbackID, on the protocols that definition supports (other
// protocols still answer 404). Replies echo the id the client asked for. It is
// opt-in: without it unknown models stay a 404, which is the contract tests and
// the production /virtual endpoint rely on. Call it before serving.
func (r *Runtime) SetFallback(fallbackID string) error {
	if _, ok := r.definitions[fallbackID]; !ok {
		return fmt.Errorf("fallback model %q is not defined", fallbackID)
	}
	r.fallback = fallbackID
	r.service.GetAnthropicRegistry().SetFallback(fallbackID)
	r.service.GetOpenAIRegistry().SetFallback(fallbackID)
	return nil
}

func New(defs []Definition) (*Runtime, error) { return NewWithCatalog(defs, NewCatalog()) }
func NewWithCatalog(defs []Definition, catalog *Catalog) (*Runtime, error) {
	if catalog == nil {
		catalog = NewCatalog()
	}
	r := &Runtime{service: virtualserver.NewEmptyService(), definitions: map[string]Definition{}, wire: map[string]scenario.Scenario{}, protocols: map[string]map[scenario.ResponseFormat]bool{}, scripts: map[string]*script.Engine{}}
	for _, original := range defs {
		// Normalize a copy, including nested config, so factories can be reused.
		raw, err := json.Marshal(original)
		if err != nil {
			return nil, err
		}
		d, err := decodeOne(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", original.ID, err)
		}
		if _, ok := r.definitions[d.ID]; ok {
			return nil, fmt.Errorf("duplicate definition id %q", d.ID)
		}
		supported := map[scenario.ResponseFormat]bool{}
		addChat := func(a anthropic.VirtualModel, o openai.VirtualModel) error {
			if a != nil {
				if err := r.service.GetAnthropicRegistry().Register(a); err != nil {
					return err
				}
				supported[scenario.FormatAnthropic] = true
			}
			if o != nil {
				if err := r.service.GetOpenAIRegistry().Register(o); err != nil {
					return err
				}
				supported[scenario.FormatOpenAIChat] = true
				supported[scenario.FormatOpenAIResponses] = true
			}
			return nil
		}
		switch d.Kind {
		case Mock:
			a := *d.Mock
			o := *d.Mock
			err = addChat(anthropic.NewMockModel(&a), openai.NewMockModel(&o))
		case Sequence:
			cfg := vmodel.SequenceConfig{ID: d.ID, Name: d.Name, Description: d.Description, Steps: d.Sequence.Steps, DefaultContent: d.Sequence.DefaultContent, OnExhaust: d.Sequence.OnExhaust, Delay: d.Sequence.Delay}
			err = addChat(anthropic.NewSequenceModel(&cfg), openai.NewSequenceModel(&cfg))
		case Decision:
			choose := decision.First
			if d.Decision.Strategy == "stable" {
				choose = decision.Stable
			}
			err = r.service.GetDecisionRegistry().Register(decision.NewMockModel(&decision.MockModelConfig{ID: d.ID, Name: d.Name, Description: d.Description, Choose: choose, Confidence: d.Decision.Confidence}))
			supported[FormatDecisions] = true
		case Transform:
			source := virtualserver.NewService().GetAnthropicRegistry().Get(d.Transform.Builtin)
			if source == nil || source.GetType() != vmodel.VirtualModelTypeProxy {
				return nil, fmt.Errorf("unknown transform %q", d.Transform.Builtin)
			}
			err = addChat(&anthropicAlias{VirtualModel: source, id: d.ID, name: d.Name}, nil)
		case Script:
			var engine *script.Engine
			engine, err = script.New(d.ID, d.Script)
			if err == nil {
				r.scripts[d.ID] = engine
				supported[scenario.FormatAnthropic] = true
			}
		case Recording:
			var sc scenario.Scenario
			sc, err = scenario.BuildRecording(scenario.Scenario{Name: d.ID, Description: d.Description, Assertions: d.Expect.Assertions(), Structural: d.Structural.Assertions(), Tags: d.Tags, SkipTransitive: d.SkipTransitive}, d.Responses)
			if err == nil {
				r.installWire(sc, supported)
			}
		case Builtin:
			if factory, ok := catalog.models[d.Builtin]; ok {
				models, e := factory(d)
				if e != nil {
					return nil, e
				}
				if models.Anthropic != nil && models.Anthropic.GetID() != d.ID {
					models.Anthropic = &anthropicAlias{VirtualModel: models.Anthropic, id: d.ID, name: d.Name}
				}
				if models.OpenAI != nil && models.OpenAI.GetID() != d.ID {
					models.OpenAI = &openaiAlias{VirtualModel: models.OpenAI, id: d.ID, name: d.Name}
				}
				if models.Decision != nil && models.Decision.GetID() != d.ID {
					models.Decision = &decisionAlias{VirtualModel: models.Decision, id: d.ID, name: d.Name}
				}
				err = addChat(models.Anthropic, models.OpenAI)
				if err == nil && models.Decision != nil {
					err = r.service.GetDecisionRegistry().Register(models.Decision)
					supported[FormatDecisions] = true
				}
				if len(supported) == 0 {
					return nil, fmt.Errorf("behavior %s returned no models", d.Builtin)
				}
			} else if factory, ok := catalog.factories[d.Builtin]; ok {
				var sc scenario.Scenario
				sc, err = factory(d)
				if err == nil {
					sc.Name = d.ID
					r.installWire(sc, supported)
				}
			} else if strings.HasPrefix(d.Builtin, "model/") {
				svc := virtualserver.NewService()
				id := strings.TrimPrefix(d.Builtin, "model/")
				var a anthropic.VirtualModel
				var o openai.VirtualModel
				if model := svc.GetAnthropicRegistry().Get(id); model != nil {
					a = &anthropicAlias{VirtualModel: model, id: d.ID, name: d.Name}
				}
				if model := svc.GetOpenAIRegistry().Get(id); model != nil {
					o = &openaiAlias{VirtualModel: model, id: d.ID, name: d.Name}
				}
				if m := svc.GetDecisionRegistry().Get(id); m != nil {
					err = r.service.GetDecisionRegistry().Register(&decisionAlias{VirtualModel: m, id: d.ID, name: d.Name})
					supported[FormatDecisions] = true
				}
				if a == nil && o == nil && !supported[FormatDecisions] {
					return nil, fmt.Errorf("unknown builtin model %q", id)
				}
				if err == nil {
					err = addChat(a, o)
				}
			} else {
				return nil, fmt.Errorf("unknown builtin behavior %q", d.Builtin)
			}
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", d.ID, err)
		}
		if len(d.Protocols) > 0 {
			selected := map[scenario.ResponseFormat]bool{}
			for _, p := range d.Protocols {
				if !supported[p] {
					return nil, fmt.Errorf("%s does not support protocol %s", d.ID, p)
				}
				selected[p] = true
			}
			supported = selected
		}
		r.definitions[d.ID] = d
		r.protocols[d.ID] = supported
	}
	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.NoRoute(r.service.GetHandler().NotSupported)
	r.service.SetupOpenAIRoutes(engine.Group("/openai"))
	r.service.SetupAnthropicRoutes(engine.Group("/anthropic"))
	r.service.SetupOpenAIRoutes(engine.Group(""))
	engine.POST("/v1/messages", r.service.GetHandler().Messages)
	r.semantic = engine
	return r, nil
}
func (r *Runtime) installWire(sc scenario.Scenario, supported map[scenario.ResponseFormat]bool) {
	r.wire[sc.Name] = sc

	for p := range sc.MockResponses {
		supported[p] = true
	}
}
func (r *Runtime) IDs() []string {
	ids := make([]string, 0, len(r.definitions))
	for id := range r.definitions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func route(path string) (string, scenario.ResponseFormat, bool) {
	for _, prefix := range []string{"/openai", "/anthropic"} {
		if strings.HasPrefix(path, prefix+"/v1/") {
			path = strings.TrimPrefix(path, prefix)
		}
	}
	switch path {
	case "/v1/chat/completions":
		return path, scenario.FormatOpenAIChat, true
	case "/v1/responses", "/codex/responses":
		return "/v1/responses", scenario.FormatOpenAIResponses, true
	case "/v1/messages":
		return path, scenario.FormatAnthropic, true
	case "/v1/decisions", "/v1/systemone":
		return path, FormatDecisions, true
	}
	if strings.HasPrefix(path, "/v1beta/models/") {
		return path, scenario.FormatGoogle, true
	}
	return path, "", false
}

func (r *Runtime) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path == "/healthz" {
		writeJSON(w, 200, map[string]string{"status": "ready"})
		return
	}
	if strings.HasPrefix(req.URL.Path, scriptControlPrefix) {
		r.serveScriptControl(w, req)
		return
	}
	if req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/v1/messages/count_tokens") && r.serveScriptCountTokens(w, req) {
		return
	}
	if req.Method == http.MethodGet && (strings.HasSuffix(req.URL.Path, "/models")) {
		switch req.URL.Path {
		case "/v1/models", "/openai/v1/models", "/anthropic/v1/models", "/v1beta/models":
		default:
			http.NotFound(w, req)
			return
		}
		models := []map[string]any{}
		for _, id := range r.IDs() {
			p := r.protocols[id]
			include := p[scenario.FormatOpenAIChat] || p[scenario.FormatOpenAIResponses] || p[FormatDecisions]
			if req.URL.Path == "/anthropic/v1/models" {
				include = p[scenario.FormatAnthropic]
			}
			if req.URL.Path == "/v1beta/models" {
				include = p[scenario.FormatGoogle]
			}
			if include {
				protocols := []string{}
				for format := range p {
					protocols = append(protocols, string(format))
				}
				sort.Strings(protocols)
				if req.URL.Path == "/v1beta/models" {
					models = append(models, map[string]any{"name": "models/" + id, "displayName": r.definitions[id].Name})
				} else {
					models = append(models, map[string]any{"id": id, "object": "model", "created": 0, "owned_by": vmodel.DefaultMockOwnedBy, "protocols": protocols})
				}
			}
		}
		if req.URL.Path == "/v1beta/models" {
			writeJSON(w, 200, map[string]any{"models": models})
		} else if req.URL.Path == "/anthropic/v1/models" {
			writeJSON(w, 200, map[string]any{"data": models, "has_more": false})
		} else {
			writeJSON(w, 200, map[string]any{"object": "list", "data": models})
		}
		return
	}

	path, format, ok := route(req.URL.Path)
	if !ok || req.Method != http.MethodPost {
		r.semantic.ServeHTTP(w, req)
		return
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		writeRequestError(w, req, 400, "read request body")
		return
	}
	var input map[string]json.RawMessage
	if err := json.Unmarshal(body, &input); err != nil || input == nil {
		writeRequestError(w, req, 400, "request must be a JSON object")
		return
	}
	var id string
	_ = json.Unmarshal(input["model"], &id)
	if format == scenario.FormatGoogle {
		id = strings.Split(strings.TrimPrefix(path, "/v1beta/models/"), ":")[0]
	}
	if _, ok := r.definitions[id]; !ok {
		if r.fallback == "" {
			writeRequestError(w, req, 404, "unknown model "+id)
			return
		}
		// Semantic models resolve the fallback in their registries; recordings
		// are keyed here.
		id = r.fallback
	}
	if !r.protocols[id][format] {
		writeRequestError(w, req, 404, "model does not support "+string(format))
		return
	}
	clone := req.Clone(req.Context())
	u := *req.URL
	u.Path = path
	clone.URL = &u
	clone.Body = io.NopCloser(bytes.NewReader(body))
	if engine, ok := r.scripts[id]; ok {
		engine.ServeHTTP(w, clone)
		return
	}
	if sc, ok := r.wire[id]; ok {
		builder := sc.MockResponses[format]
		var streaming bool
		_ = json.Unmarshal(input["stream"], &streaming)
		if format == scenario.FormatGoogle {
			streaming = strings.Contains(path, "streamGenerateContent")
		}
		if streaming && builder.StreamHTTPError < 400 && builder.Stream == nil && builder.StreamFor == nil {
			writeRequestError(w, req, 400, "model has no streaming recording")
			return
		}
		if !streaming && builder.NonStream == nil && builder.NonStreamFor == nil {
			writeRequestError(w, req, 400, "model has no non-stream recording")
			return
		}
		benchmark.WriteBuilderResponse(w, builder, streaming, body)
		return
	}
	r.semantic.ServeHTTP(w, clone)
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeRequestError(w http.ResponseWriter, req *http.Request, status int, message string) {
	_, format, _ := route(req.URL.Path)
	if format == scenario.FormatAnthropic {
		writeJSON(w, status, map[string]any{"type": "error", "error": map[string]string{"message": message, "type": "invalid_request_error"}})
		return
	}
	if format == scenario.FormatGoogle {
		code := "INVALID_ARGUMENT"
		if status == 404 {
			code = "NOT_FOUND"
		}
		writeJSON(w, status, map[string]any{"error": map[string]any{"code": status, "message": message, "status": code}})
		return
	}
	writeJSON(w, status, map[string]any{"error": map[string]string{"message": message, "type": "vmodel_error"}})
}

// Scenario adapts the same compiled runtime to the existing protocol matrix.
// Native Go callbacks remain callbacks; generated models are rendered by the
// production handler rather than duplicated protocol templates.
func (r *Runtime) Scenario(id string) (scenario.Scenario, error) {
	d, ok := r.definitions[id]
	if !ok {
		return scenario.Scenario{}, fmt.Errorf("unknown model %q", id)
	}
	if d.Kind == Script {
		return scenario.Scenario{}, fmt.Errorf("%s is a script: it is served to agent clients, not run in the matrix", id)
	}
	if sc, ok := r.wire[id]; ok {
		if d.Description != "" {
			sc.Description = d.Description
		}
		if d.Tags != nil {
			sc.Tags = d.Tags
		}
		sc.SkipTransitive = d.SkipTransitive
		if d.Expect != nil {
			sc.Assertions = d.Expect.Assertions()
		}
		if d.Structural != nil {
			sc.Structural = d.Structural.Assertions()
		}
		responses := map[scenario.ResponseFormat]scenario.MockResponseBuilder{}
		for p, b := range sc.MockResponses {
			if r.protocols[id][p] {
				responses[p] = b
			}
		}
		sc.MockResponses = responses
		compatible := false
		for p := range responses {
			if p != FormatDecisions {
				compatible = true
			}
		}
		if !compatible {
			return scenario.Scenario{}, fmt.Errorf("%s has no chat matrix protocol; test its native endpoint", id)
		}
		return sc, nil
	}
	sc := scenario.Scenario{Name: id, Description: d.Description, Tags: d.Tags, SkipTransitive: d.SkipTransitive, Assertions: d.Expect.Assertions(), Structural: d.Structural.Assertions(), MockResponses: map[scenario.ResponseFormat]scenario.MockResponseBuilder{}}
	for format := range r.protocols[id] {
		f := format
		if f == FormatDecisions {
			continue
		} // Decisions has its own request driver, not a chat matrix pair.
		builder := scenario.MockResponseBuilder{}
		builder.NonStreamFor = func(request []byte) (int, []byte) { return r.render(id, f, request, false) }
		builder.NonStream = func() (int, []byte) { return r.render(id, f, nil, false) }
		builder.StreamFor = func(request []byte) []string {
			_, raw := r.render(id, f, request, true)
			var lines []string
			for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
				if line != "" {
					lines = append(lines, line)
				}
			}
			return lines
		}
		builder.Stream = func() []string { return builder.StreamFor(nil) }
		// Matrix pre-content errors must retain their HTTP status, not become SSE 200.
		if d.Kind == Mock && d.Mock.Error != nil && d.Mock.Error.Stage == vmodel.ErrorStagePreContent {
			builder.StreamHTTPError = d.Mock.Error.Status
			if builder.StreamHTTPError == 0 {
				builder.StreamHTTPError = 500
			}
		}
		sc.MockResponses[f] = builder
	}
	if len(sc.MockResponses) == 0 {
		return scenario.Scenario{}, fmt.Errorf("%s has no chat/recording matrix protocol; test its native endpoint", id)
	}
	return sc, nil
}

// Gin's streaming API expects CloseNotifier even when rendering in memory.
type streamRecorder struct{ *httptest.ResponseRecorder }

func (w streamRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	a, b := net.Pipe()
	_ = b.Close()
	return a, bufio.NewReadWriter(bufio.NewReader(a), bufio.NewWriter(a)), nil
}
func (w streamRecorder) CloseNotify() <-chan bool { return make(chan bool) }
func (r *Runtime) render(id string, format scenario.ResponseFormat, raw []byte, stream bool) (int, []byte) {
	input := map[string]json.RawMessage{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &input); err != nil {
			return 400, []byte(`{"error":"invalid request"}`)
		}
	}
	input["model"], _ = json.Marshal(id)
	input["stream"], _ = json.Marshal(stream)
	if len(raw) == 0 {
		input["messages"] = json.RawMessage(`[{"role":"user","content":"hello"}]`)
		input["max_tokens"] = json.RawMessage(`1024`)
	}
	path := "/v1/chat/completions"
	if format == scenario.FormatAnthropic {
		path = "/v1/messages"
	}
	if format == scenario.FormatOpenAIResponses {
		path = "/v1/responses"
		if len(raw) == 0 {
			input["input"] = json.RawMessage(`"hello"`)
			delete(input, "messages")
		}
	}
	body, err := json.Marshal(input)
	if err != nil {
		return 400, []byte(`{"error":"invalid request"}`)
	}
	req := httptest.NewRequest("POST", path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(streamRecorder{rec}, req)
	return rec.Code, rec.Body.Bytes()
}

// Aliases keep a referenced model's behavior while giving it the definition's identity.
type anthropicAlias struct {
	anthropic.VirtualModel
	id, name string
}

func (m *anthropicAlias) GetID() string         { return m.id }
func (m *anthropicAlias) GetName() string       { return m.name }
func (m *anthropicAlias) ToModel() vmodel.Model { v := m.VirtualModel.ToModel(); v.ID = m.id; return v }
func (m *anthropicAlias) Snapshot() anthropic.VirtualModel {
	return &anthropicAlias{VirtualModel: anthropic.Snapshot(m.VirtualModel), id: m.id, name: m.name}
}
func (m *anthropicAlias) ErrorInjection() *vmodel.ErrorInjection {
	return vmodel.ExtractErrorInjection(m.VirtualModel)
}

type openaiAlias struct {
	openai.VirtualModel
	id, name string
}

func (m *openaiAlias) GetID() string         { return m.id }
func (m *openaiAlias) GetName() string       { return m.name }
func (m *openaiAlias) ToModel() vmodel.Model { v := m.VirtualModel.ToModel(); v.ID = m.id; return v }
func (m *openaiAlias) Snapshot() openai.VirtualModel {
	return &openaiAlias{VirtualModel: openai.Snapshot(m.VirtualModel), id: m.id, name: m.name}
}
func (m *openaiAlias) ErrorInjection() *vmodel.ErrorInjection {
	return vmodel.ExtractErrorInjection(m.VirtualModel)
}

type decisionAlias struct {
	decision.VirtualModel
	id, name string
}

func (m *decisionAlias) GetID() string         { return m.id }
func (m *decisionAlias) GetName() string       { return m.name }
func (m *decisionAlias) ToModel() vmodel.Model { v := m.VirtualModel.ToModel(); v.ID = m.id; return v }

// Check applies the file's expectations to a native endpoint result. JSON
// field and raw-body checks work for decisions and recordings as well as chat.
func (r *Runtime) Check(id string, result *check.RoundTripResult) error {
	d, ok := r.definitions[id]
	if !ok {
		return fmt.Errorf("unknown model %q", id)
	}
	if result == nil {
		return fmt.Errorf("test result is required")
	}
	assertions := d.Expect.Assertions()
	if d.Expect == nil {
		if sc, ok := r.wire[id]; ok {
			assertions = sc.Assertions
		}
	}
	if len(assertions) == 0 {
		return fmt.Errorf("model %s has no expectations", id)
	}
	var failures []error
	for _, failure := range check.Evaluate(result, assertions) {
		failures = append(failures, fmt.Errorf("%s: %w", failure.Name, failure.Err))
	}
	return errors.Join(failures...)
}
