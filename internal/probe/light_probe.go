package probe

import (
	"context"
	"fmt"
	"time"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// LightProber runs the optional "Test Connection" probe used when a
// user adds an API key. It pokes OPTIONS, /models, /chat/completions, and
// /responses and returns a per-endpoint report; results are advisory only
// and do not block onboarding. Independent of *Server.
type LightProber struct {
	pool *client.ClientPool
}

// NewLightProber constructs a LightProber backed by the given client pool.
func NewLightProber(pool *client.ClientPool) *LightProber {
	return &LightProber{pool: pool}
}

// Probe runs every applicable sub-probe for the provider and returns a
// populated LightweightProbeResponseData. Never returns an error — partial
// failure is encoded in the per-endpoint fields and the Valid summary.
func (l *LightProber) Probe(ctx context.Context, provider *typ.Provider) *LightweightProbeResponseData {
	data := &LightweightProbeResponseData{
		Provider: provider.Name,
		APIBase:  provider.APIBase,
		APIStyle: string(provider.APIStyle),
	}

	// Each helper writes its outcome directly into data. Track the count of
	// endpoints actually run so the summary denominator is correct (non-OpenAI
	// providers skip chat/responses).
	l.runOptionsEndpoint(ctx, provider,
		&data.OptionsSuccess, &data.OptionsMessage, &data.OptionsResponseTime)
	l.runModelsEndpoint(ctx, provider,
		&data.ModelsSuccess, &data.ModelsMessage, &data.ModelsResponseTime, &data.ModelsCount, &data.Warning)
	ran := 2

	if provider.APIStyle == protocol.APIStyleOpenAI {
		l.runChatEndpoint(ctx, provider,
			&data.ChatSuccess, &data.ChatMessage, &data.ChatResponseTime)
		l.runResponsesEndpoint(ctx, provider,
			&data.ResponsesSuccess, &data.ResponsesMessage, &data.ResponsesResponseTime)
		ran = 4
	}

	successes := 0
	for _, ok := range []bool{data.OptionsSuccess, data.ModelsSuccess, data.ChatSuccess, data.ResponsesSuccess} {
		if ok {
			successes++
		}
	}
	data.Valid = successes > 0
	if data.Valid {
		data.Message = fmt.Sprintf("Connection test completed - %d/%d endpoints accessible", successes, ran)
	} else {
		data.Message = "Connection test failed - unable to reach any provider endpoint"
	}

	return data
}

// runOptionsEndpoint issues a bare OPTIONS request (HTTP-level, no SDK) and
// writes the outcome into the target fields.
func (l *LightProber) runOptionsEndpoint(ctx context.Context, provider *typ.Provider,
	success *bool, msg *string, rt *int64) {
	switch provider.APIStyle {
	case protocol.APIStyleOpenAI, protocol.APIStyleAnthropic, protocol.APIStyleGoogle:
		// supported below
	default:
		*success, *msg, *rt = false, fmt.Sprintf("Unsupported API style: %s", provider.APIStyle), 0
		return
	}
	start := time.Now()
	r := probeOptions(ctx, provider)
	*rt = time.Since(start).Milliseconds()
	if r.Success {
		*success, *msg = true, "OPTIONS request successful"
	} else {
		*success, *msg = false, fmt.Sprintf("OPTIONS failed: %s", r.ErrorMessage)
	}
}

// runChatEndpoint and runResponsesEndpoint run a minimal SDK round-trip against
// the respective OpenAI endpoint and write the outcome into the target fields.
// They share the timing/client/timeout boilerplate; only the call differs.
func (l *LightProber) runChatEndpoint(ctx context.Context, provider *typ.Provider,
	success *bool, msg *string, rt *int64) {
	l.runOpenAIEndpoint(ctx, provider, success, msg, rt, "Chat endpoint accessible",
		func(c client.OpenAIClientInterface, pctx context.Context) (*Result, error) {
			return probeOpenAIChat(pctx, c, "gpt-3.5-turbo", "Hi", E2EModeSimple)
		})
}

func (l *LightProber) runResponsesEndpoint(ctx context.Context, provider *typ.Provider,
	success *bool, msg *string, rt *int64) {
	l.runOpenAIEndpoint(ctx, provider, success, msg, rt, "Responses API endpoint accessible",
		func(c client.OpenAIClientInterface, pctx context.Context) (*Result, error) {
			return probeOpenAIResponses(pctx, c, "gpt-4o", "Hi", E2EModeSimple)
		})
}

// runOpenAIEndpoint is the shared body for chat/responses connectivity checks.
// okLabel is the success message; call dispatches the actual probe.
func (l *LightProber) runOpenAIEndpoint(ctx context.Context, provider *typ.Provider,
	success *bool, msg *string, rt *int64, okLabel string,
	call func(c client.OpenAIClientInterface, pctx context.Context) (*Result, error)) {
	start := time.Now()
	c := l.pool.GetOpenAIClient(context.Background(), provider, "")
	if c == nil {
		*success, *msg, *rt = false, "Failed to create OpenAI client", 0
		return
	}
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	res, err := call(c, probeCtx)
	*rt = time.Since(start).Milliseconds()
	switch {
	case err != nil:
		*success, *msg = false, fmt.Sprintf("Endpoint failed: %v", err)
	case res != nil && res.Success:
		*success, *msg = true, okLabel
	default:
		*success, *msg = false, "Endpoint returned no content"
	}
}

// runModelsEndpoint runs the /models list probe and writes its outcome (incl.
// model count and any warning) into the target fields.
func (l *LightProber) runModelsEndpoint(ctx context.Context, provider *typ.Provider,
	success *bool, msg *string, rt *int64, count *int, warning *string) {
	start := time.Now()
	report := func(ok bool, message string, models int, warn string) {
		*success, *msg, *rt, *count, *warning = ok, message, time.Since(start).Milliseconds(), models, warn
	}

	var lister client.ModelLister
	switch provider.APIStyle {
	case protocol.APIStyleOpenAI:
		c := l.pool.GetOpenAIClient(context.Background(), provider, "")
		lister = c
	case protocol.APIStyleAnthropic:
		c := l.pool.GetAnthropicClient(context.Background(), provider, "")
		lister = c
	case protocol.APIStyleGoogle:
		c := l.pool.GetGoogleClient(context.Background(), provider, "")
		lister = c
	default:
		report(false, fmt.Sprintf("Unsupported API style: %s", provider.APIStyle), 0, "")
		return
	}
	if lister == nil {
		report(false, fmt.Sprintf("Failed to create %s client", provider.APIStyle), 0, "")
		return
	}

	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	models, err := lister.ListModels(probeCtx)
	switch {
	case client.IsModelsEndpointNotSupported(err):
		report(false, "Models endpoint not supported for this provider type", 0,
			"This provider does not support the models list endpoint (e.g., OAuth-based providers)")
	case err != nil:
		report(false, fmt.Sprintf("Models endpoint failed: %v", err), 0, "")
	case len(models) == 0:
		report(false, "Models endpoint returned no models", 0, "")
	default:
		report(true, fmt.Sprintf("Models endpoint accessible - %d models found", len(models)), len(models), "")
	}
}
