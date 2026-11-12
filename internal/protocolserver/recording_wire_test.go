package protocolserver

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/recording"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type wireTestExporter struct{ records chan *obs.Record }

func (e *wireTestExporter) Export(_ context.Context, records []*obs.Record) error {
	for _, record := range records {
		e.records <- record
	}
	return nil
}
func (*wireTestExporter) Shutdown(context.Context) error { return nil }

func TestAnthropicWireRecorder(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	exporter := &wireTestExporter{records: make(chan *obs.Record, 1)}
	sink := obs.NewSink("", obs.RecordModeRequestOnly, obs.WithExporters(exporter))
	recorder, err := recording.NewProtocolRecorder(c, sink, "test", obs.RecordModeRequestOnly, []byte(`{}`))
	require.NoError(t, err)
	c.Set(constant.CtxKeyProtocolRecorder, recorder)
	require.Nil(t, anthropicWireRecorder(c), "flag disabled must keep the regular recording path")
	c.Request = c.Request.WithContext(typ.WithRuleFlags(c.Request.Context(), typ.RuleFlags{DisableThinkingBudget: true}))
	callback := anthropicWireRecorder(c)
	require.NotNil(t, callback)
	req := &anthropic.MessageNewParams{Thinking: anthropic.ThinkingConfigParamOfEnabled(4096)}
	req.Thinking.OfEnabled.SetExtraFields(map[string]any{"budget_tokens": param.Omit})
	require.NoError(t, callback(req))
	recorder.RecordResponse(nil, "model")
	require.NoError(t, sink.ForceFlush(context.Background()))
	record := <-exporter.records
	require.NotNil(t, record.TransformedRequest)
	thinking, ok := record.TransformedRequest.Body["thinking"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "enabled", thinking["type"])
	require.NotContains(t, thinking, "budget_tokens")
}
