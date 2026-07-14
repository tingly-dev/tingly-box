package obs

import (
	"context"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	requestrecord "github.com/tingly-dev/tingly-box/internal/record"
)

func TestSinkEmitRequestRecordUsesExistingPipeline(t *testing.T) {
	exporter := &recordingExporter{}
	sink := NewSink("", RecordModeStagedRequestResponse, WithExporters(exporter))
	if sink == nil {
		t.Fatal("NewSink returned nil")
	}
	t.Cleanup(sink.Close)

	started := time.Now().UTC()
	requestRecord := &requestrecord.RequestRecord{
		Timestamp: started,
		RequestID: "request-id",
		SessionID: "session-id",
		Scenario:  "claude_code",
		Outcome:   requestrecord.OutcomeSucceeded,
		Duration:  time.Second,
		InputRequest: requestrecord.Payload{
			Protocol: protocol.TypeAnthropicBeta,
		},
		ProviderExchanges: []requestrecord.ProviderExchange{{
			Provider: "provider",
			Model:    "provider-model",
			Protocol: protocol.TypeAnthropicBeta,
		}},
	}

	sink.EmitRequestRecord(requestRecord)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sink.ForceFlush(ctx); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}
	if len(exporter.batches) != 1 || len(exporter.batches[0]) != 1 {
		t.Fatalf("exported batches = %#v", exporter.batches)
	}

	got := exporter.batches[0][0]
	if got.RequestRecord != requestRecord {
		t.Fatal("request record envelope was not preserved")
	}
	if got.Provider != "provider" || got.Model != "provider-model" {
		t.Fatalf("provider/model = %q/%q", got.Provider, got.Model)
	}
	if full := FullRecord(got); full.RequestRecord != requestRecord {
		t.Fatal("full exporter shape dropped request_record")
	}
}
