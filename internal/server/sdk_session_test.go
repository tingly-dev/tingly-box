package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestScenarioTransportLabel(t *testing.T) {
	cases := []struct {
		name      string
		transport []typ.ScenarioTransport
		want      string
	}{
		{"both", []typ.ScenarioTransport{typ.TransportOpenAI, typ.TransportAnthropic}, "both"},
		{"anthropic-only", []typ.ScenarioTransport{typ.TransportAnthropic}, "anthropic"},
		{"openai-only", []typ.ScenarioTransport{typ.TransportOpenAI}, "openai"},
		{"embed-falls-to-openai", []typ.ScenarioTransport{typ.TransportEmbed}, "openai"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scenarioTransportLabel(typ.ScenarioDescriptor{SupportedTransport: tc.transport})
			if got != tc.want {
				t.Fatalf("scenarioTransportLabel(%v) = %q, want %q", tc.transport, got, tc.want)
			}
		})
	}
}

func TestBindableScenarioIDsIncludesExperiment(t *testing.T) {
	ids := bindableScenarioIDs()
	found := false
	for _, id := range ids {
		if id == string(typ.ScenarioExperiment) {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected experiment scenario in bindable list, got %v", ids)
	}
}

// TestSDKSessionResponseShape freezes the JSON field names the Python SDK
// depends on. If a field is renamed here without updating the SDK, this fails.
func TestSDKSessionResponseShape(t *testing.T) {
	resp := SDKSessionResponse{
		BaseURL:   "http://127.0.0.1:12580/tingly/experiment",
		Token:     "tok",
		Scenario:  "experiment",
		Transport: "both",
		Ready:     true,
		Services:  2,
	}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"base_url", "token", "scenario", "transport", "ready", "services"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("response JSON missing %q field; have %v", key, m)
		}
	}
	// expires_at is omitempty and absent here
	if _, ok := m["expires_at"]; ok {
		t.Fatalf("expires_at should be omitted when empty")
	}
}

// TestCreateSDKSessionWritesTheDeclaredShape checks the thing the test above
// cannot: that the HANDLER writes what the route's swagger annotation declares.
//
// The shape test marshals the struct, so it passes whether or not the handler
// wraps the payload in a {success, data} envelope — and for a while the handler
// did wrap it, which meant openapi.json described a body the endpoint never
// sent. That matters now that the Python SDK's models are generated straight
// off openapi.json: a wrapper here becomes a validation error there.
//
// DisallowUnknownFields is the actual assertion — an envelope shows up as an
// unknown "success"/"data" key rather than as a missing field.
func TestCreateSDKSessionWritesTheDeclaredShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &Server{config: &config.Config{}}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/sdk/session",
		strings.NewReader(`{"scenario":"experiment","name":"t"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	s.CreateSDKSession(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	dec := json.NewDecoder(strings.NewReader(w.Body.String()))
	dec.DisallowUnknownFields()
	var got SDKSessionResponse
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("response body does not decode into the declared SDKSessionResponse "+
			"(an envelope or an extra field would cause this): %v\nbody=%s", err, w.Body.String())
	}
	if got.Scenario != string(typ.ScenarioExperiment) {
		t.Fatalf("scenario = %q, want %q", got.Scenario, typ.ScenarioExperiment)
	}
	if got.Transport == "" {
		t.Fatal("transport should be populated")
	}
}
