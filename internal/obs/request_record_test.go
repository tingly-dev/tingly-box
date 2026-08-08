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
	if got.RequestRecord == requestRecord {
		t.Fatal("request record was not detached before asynchronous export")
	}
	if got.Provider != "provider" || got.Model != "provider-model" {
		t.Fatalf("provider/model = %q/%q", got.Provider, got.Model)
	}
	if full := FullRecord(got); full.RequestRecord != got.RequestRecord {
		t.Fatal("full exporter shape dropped request_record")
	}
}

func TestSinkEmitRequestRecordHonorsRecordingMode(t *testing.T) {
	for _, testCase := range []struct {
		name                 string
		mode                 RecordMode
		wantProviderResponse bool
		wantFinalResponse    bool
	}{
		{name: "request", mode: RecordModeRequestOnly},
		{name: "request response", mode: RecordModeRequestResponse, wantFinalResponse: true},
		{name: "staged", mode: RecordModeStagedRequestResponse, wantProviderResponse: true, wantFinalResponse: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			exporter := &recordingExporter{}
			sink := NewSink("", testCase.mode, WithExporters(exporter))
			t.Cleanup(sink.Close)
			requestRecord := &requestrecord.RequestRecord{
				Timestamp: time.Now().UTC(),
				InputRequest: requestrecord.Payload{
					Protocol: protocol.TypeOpenAIChat,
				},
				ProviderExchanges: []requestrecord.ProviderExchange{{
					Protocol: protocol.TypeOpenAIChat,
					Request:  requestrecord.Payload{Protocol: protocol.TypeOpenAIChat},
					Response: &requestrecord.Payload{Protocol: protocol.TypeOpenAIChat},
				}},
				FinalResponse: &requestrecord.Payload{Protocol: protocol.TypeOpenAIChat},
			}

			sink.EmitRequestRecord(requestRecord)
			requestRecord.InputRequest.Body = []byte(`{"mutated":true}`)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := sink.ForceFlush(ctx); err != nil {
				t.Fatalf("ForceFlush: %v", err)
			}
			got := exporter.batches[0][0].RequestRecord
			if string(got.InputRequest.Body) == string(requestRecord.InputRequest.Body) {
				t.Fatal("exported record retained caller-owned payload storage")
			}
			if (got.ProviderExchanges[0].Response != nil) != testCase.wantProviderResponse {
				t.Fatalf("provider response present = %v, want %v", got.ProviderExchanges[0].Response != nil, testCase.wantProviderResponse)
			}
			if (got.FinalResponse != nil) != testCase.wantFinalResponse {
				t.Fatalf("final response present = %v, want %v", got.FinalResponse != nil, testCase.wantFinalResponse)
			}
			if requestRecord.ProviderExchanges[0].Response == nil || requestRecord.FinalResponse == nil {
				t.Fatal("mode projection mutated the completed RequestRecord")
			}
		})
	}
}
