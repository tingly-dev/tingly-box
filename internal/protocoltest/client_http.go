package protocoltest

import (
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// httpClient is the default driver: hand-crafted JSON over Go's net/http
// (the original harness behavior). Non-streaming requests go through the gin
// engine directly via httptest.ResponseRecorder; streaming uses the real
// httptest server. See TestEnv.dispatch.
type httpClient struct{}

// NewHTTPClient returns the default raw-HTTP client driver.
func NewHTTPClient() Client { return &httpClient{} }

func (c *httpClient) Name() string { return "http" }

func (c *httpClient) Supports(protocol.APIType) bool { return true }

func (c *httpClient) Send(env *TestEnv, spec SendSpec) (*RoundTripResult, error) {
	path, body := buildRequest(spec.Source, spec.RequestModel, spec.Streaming)
	return env.dispatch(spec.Source, spec.Target, spec.ScenarioName, path, body, nil, spec.Streaming)
}
