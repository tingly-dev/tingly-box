package scenario

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/tidwall/gjson"
	"os"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tingly-dev/tingly-box/vmodel"
	"github.com/tingly-dev/tingly-box/vmodel/benchmark/check"
)

// Fixture is the file representation of an existing Scenario. Recorded wire
// responses remain protocol-native; loading only adapts data to its builders.
type Fixture struct {
	Name           string                      `json:"name"`
	Description    string                      `json:"description,omitempty"`
	Tags           []string                    `json:"tags,omitempty"`
	MockResponses  map[ResponseFormat]Response `json:"mock_responses"`
	Expect         *Expect                     `json:"expect,omitempty"`
	Structural     *Expect                     `json:"structural,omitempty"`
	SkipTransitive bool                        `json:"skip_transitive,omitempty"`
}

// MarshalYAML preserves the JSON field names and encodes recorded JSON bodies
// as YAML values rather than exposing json.RawMessage's byte representation.
func (f Fixture) MarshalYAML() (any, error) {
	raw, err := json.Marshal(f)
	if err != nil {
		return nil, err
	}
	var node yaml.Node
	if err := yaml.Unmarshal(raw, &node); err != nil {
		return nil, err
	}
	return node.Content[0], nil
}

// Response stores either JSON body data or an exact raw body, plus SSE lines in
// the same representation as MockResponseBuilder.Stream. Terminal events are
// never added automatically: incomplete/error recordings must stay incomplete.
type Response struct {
	Status          int             `json:"status,omitempty"`
	Body            json.RawMessage `json:"body,omitempty"`
	RawBody         *string         `json:"raw_body,omitempty"`
	RawStream       *string         `json:"raw_stream,omitempty"`
	Stream          []string        `json:"stream,omitempty"`
	StreamHTTPError int             `json:"stream_http_error,omitempty"`
}

// Expect maps a small declarative vocabulary onto the existing check library.
// Pointers distinguish an omitted assertion from an explicit empty/zero value.
type Expect struct {
	JSONFields      map[string]json.RawMessage `json:"json_fields,omitempty"`
	BodyContains    *string                    `json:"body_contains,omitempty"`
	HTTPStatus      *int                       `json:"http_status,omitempty"`
	ContentEquals   *string                    `json:"content_equals,omitempty"`
	ContentContains *string                    `json:"content_contains,omitempty"`
	FinishReason    *string                    `json:"finish_reason,omitempty"`
	ToolCallCount   *int                       `json:"tool_call_count,omitempty"`
	ContentNonEmpty bool                       `json:"content_non_empty,omitempty"`
	StreamEvents    []string                   `json:"stream_events,omitempty"`
}

func (e *Expect) Assertions() []check.Assertion {
	if e == nil {
		return nil
	}
	var out []check.Assertion
	if e.BodyContains != nil {
		want := *e.BodyContains
		out = append(out, check.Assertion{Name: "body_contains", Check: func(r *check.RoundTripResult) error {
			if !strings.Contains(string(r.RawBody), want) {
				return fmt.Errorf("body does not contain %q", want)
			}
			return nil
		}})
	}
	keys := make([]string, 0, len(e.JSONFields))
	for path := range e.JSONFields {
		keys = append(keys, path)
	}
	sort.Strings(keys)
	for _, path := range keys {
		expected := append(json.RawMessage(nil), e.JSONFields[path]...)
		out = append(out, check.Assertion{Name: "json_field(" + path + ")", Check: func(r *check.RoundTripResult) error {
			if !json.Valid(r.RawBody) {
				return fmt.Errorf("response body is not JSON")
			}
			actual := gjson.GetBytes(r.RawBody, path)
			if !actual.Exists() {
				return fmt.Errorf("missing JSON field %s", path)
			}
			decode := func(raw []byte) (any, error) {
				var value any
				decoder := json.NewDecoder(bytes.NewReader(raw))
				decoder.UseNumber()
				err := decoder.Decode(&value)
				return value, err
			}
			want, err := decode(expected)
			if err != nil {
				return err
			}
			got, err := decode([]byte(actual.Raw))
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(want, got) {
				return fmt.Errorf("JSON field %s: got %s, want %s", path, actual.Raw, expected)
			}
			return nil
		}})
	}
	if e.HTTPStatus != nil {
		out = append(out, check.AssertHTTPStatus(*e.HTTPStatus))
	}
	if e.ContentEquals != nil {
		out = append(out, check.AssertContentEquals(*e.ContentEquals))
	}
	if e.ContentContains != nil {
		out = append(out, check.AssertContentContains(*e.ContentContains))
	}
	if e.FinishReason != nil {
		out = append(out, check.AssertFinishReason(*e.FinishReason))
	}
	if e.ToolCallCount != nil {
		out = append(out, check.AssertHasToolCalls(*e.ToolCallCount))
	}
	if e.ContentNonEmpty {
		out = append(out, check.AssertContentNonEmpty())
	}
	if len(e.StreamEvents) > 0 {
		out = append(out, check.AssertStreamEventsContain(e.StreamEvents...))
	}
	return out
}

// LoadFixture loads JSON or YAML into the same Scenario used by the benchmark
// and protocoltest registries. It does not modify any default registry.
// Deprecated: use vmodel/spec.Load and Runtime.Scenario for unified definitions.
func LoadFixture(path string) (Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Scenario{}, err
	}
	s, err := DecodeFixture(data)
	if err != nil {
		return Scenario{}, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

func DecodeFixture(data []byte) (Scenario, error) {
	var fixture Fixture
	if err := vmodel.DecodeConfig(data, &fixture); err != nil {
		return Scenario{}, err
	}
	return fixture.Build()
}

// Build creates immutable builder snapshots, so callers can reuse a fixture
// without sharing mutable body buffers with the running server.
func (f Fixture) Build() (Scenario, error) {
	if strings.TrimSpace(f.Name) == "" {
		return Scenario{}, fmt.Errorf("name is required")
	}
	if len(f.MockResponses) == 0 {
		return Scenario{}, fmt.Errorf("mock_responses is required")
	}
	s := Scenario{
		Name: f.Name, Description: f.Description, Tags: append([]string(nil), f.Tags...),
		Assertions: f.Expect.Assertions(), Structural: f.Structural.Assertions(),
		SkipTransitive: f.SkipTransitive,
		MockResponses:  make(map[ResponseFormat]MockResponseBuilder, len(f.MockResponses)),
	}
	for format, response := range f.MockResponses {
		switch format {
		case FormatOpenAIChat, FormatOpenAIResponses, FormatAnthropic, FormatGoogle, ResponseFormat("decisions"):
		default:
			return Scenario{}, fmt.Errorf("unknown mock_responses format %q", format)
		}
		builder, err := response.builder()
		if err != nil {
			return Scenario{}, fmt.Errorf("mock_responses.%s: %w", format, err)
		}
		s.MockResponses[format] = builder
	}
	return s, nil
}

func (r Response) builder() (MockResponseBuilder, error) {
	if r.Status == 0 {
		r.Status = 200
		if r.StreamHTTPError != 0 {
			r.Status = r.StreamHTTPError
		}
	}
	if r.Status < 100 || r.Status > 599 {
		return MockResponseBuilder{}, fmt.Errorf("status must be 100..599")
	}
	if r.RawBody != nil && len(r.Body) > 0 {
		return MockResponseBuilder{}, fmt.Errorf("body and raw_body are mutually exclusive")
	}
	if len(r.Body) > 0 && !json.Valid(r.Body) {
		return MockResponseBuilder{}, fmt.Errorf("body must be valid JSON")
	}
	if r.StreamHTTPError != 0 && (r.StreamHTTPError < 400 || r.StreamHTTPError > 599) {
		return MockResponseBuilder{}, fmt.Errorf("stream_http_error must be 400..599")
	}
	if r.StreamHTTPError != 0 && r.StreamHTTPError != r.Status {
		return MockResponseBuilder{}, fmt.Errorf("stream_http_error must match status")
	}
	body := append([]byte(nil), r.Body...)
	if r.RawBody != nil {
		body = []byte(*r.RawBody)
	}
	if r.RawStream != nil && r.Stream != nil {
		return MockResponseBuilder{}, fmt.Errorf("stream and raw_stream are mutually exclusive")
	}
	if body == nil && r.Stream == nil && r.RawStream == nil {
		return MockResponseBuilder{}, fmt.Errorf("body, raw_body, or stream is required")
	}
	if r.StreamHTTPError != 0 && body == nil {
		return MockResponseBuilder{}, fmt.Errorf("stream_http_error requires a body")
	}
	b := MockResponseBuilder{StreamHTTPError: r.StreamHTTPError}
	if body != nil {
		status := r.Status
		b.NonStream = func() (int, []byte) { return status, append([]byte(nil), body...) }
	}
	// The matrix detects streaming support from this callback's presence.
	// A pre-content HTTP failure supports streaming requests without SSE data.
	if r.RawStream != nil {
		raw := []byte(*r.RawStream)
		b.RawStream = func() []byte { return append([]byte(nil), raw...) }
		b.Stream = func() []string { return []string{string(raw)} }
	}
	if r.Stream != nil || (r.StreamHTTPError != 0 && r.RawStream == nil) {
		lines := append([]string(nil), r.Stream...)
		b.Stream = func() []string { return append([]string(nil), lines...) }
	}
	return b, nil
}
