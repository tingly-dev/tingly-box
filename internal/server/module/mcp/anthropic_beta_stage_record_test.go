package mcp

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/assembler"
	protocolstage "github.com/tingly-dev/tingly-box/internal/protocol/stage"
	"github.com/tingly-dev/tingly-box/internal/record"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
)

func TestAnthropicBetaToolLoopRecordingComplete(t *testing.T) {
	request := &anthropic.BetaMessageNewParams{
		Model:     "client",
		MaxTokens: 64,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hello"))},
	}
	recorder := newBetaStageRecorder(t, "beta-complete", request)
	terminal := &betaStageScriptedEndpoint{responses: []*protocolstage.Response{
		{Value: betaStageToolMessage(t, betaStageToolCallSpec{ID: "toolu-record", Name: "lookup"})},
		{Value: betaStageTextMessage(t, "recorded final")},
	}}
	observed := record.ObserveProvider(terminal, recorder, record.ExchangeMetadata{
		Attempt: 4, Provider: "provider-a", Model: "provider-model",
	})
	endpoint := composeRecordedBetaToolLoop(t, observed)

	response, err := endpoint.Complete(context.Background(), protocolstage.Call{Request: request})
	if err != nil {
		t.Fatal(err)
	}
	if err := recorder.SetFinalResponse(protocol.TypeAnthropicBeta, response.Value); err != nil {
		t.Fatal(err)
	}
	completed, first := recorder.Finish(nil)
	if !first {
		t.Fatal("recorder was already finished")
	}
	assertBetaStageToolLoopRecord(t, completed, "toolu-record", "recorded final")
}

func TestAnthropicBetaToolLoopRecordingStream(t *testing.T) {
	request := &anthropic.BetaMessageNewParams{Model: "client", MaxTokens: 64}
	recorder := newBetaStageRecorder(t, "beta-stream", request)
	terminal := &betaStageScriptedEndpoint{streams: []*betaStageMemoryStream{
		{events: betaStageToolStreamEvents(betaStageToolCallSpec{ID: "toolu-record-stream", Name: "lookup"})},
		{events: betaStageTextStreamEvents("recorded stream final")},
	}}
	observed := record.ObserveProvider(terminal, recorder, record.ExchangeMetadata{
		Attempt: 4, Provider: "provider-a", Model: "provider-model",
	})
	endpoint := composeRecordedBetaToolLoop(t, observed)

	stream, err := endpoint.Stream(context.Background(), protocolstage.Call{Request: request})
	if err != nil {
		t.Fatal(err)
	}
	finalAssembler, err := assembler.NewStreamAssembler(protocol.TypeAnthropicBeta)
	if err != nil {
		t.Fatal(err)
	}
	for {
		event, nextErr := stream.Next(context.Background())
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			t.Fatal(nextErr)
		}
		if err := finalAssembler.Add(event.Value); err != nil {
			t.Fatal(err)
		}
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	finalResponse, err := finalAssembler.Finish()
	if err != nil {
		t.Fatal(err)
	}
	if err := recorder.SetFinalResponse(protocol.TypeAnthropicBeta, finalResponse); err != nil {
		t.Fatal(err)
	}
	completed, first := recorder.Finish(nil)
	if !first {
		t.Fatal("recorder was already finished")
	}
	assertBetaStageToolLoopRecord(t, completed, "toolu-record-stream", "recorded stream final")
}

func newBetaStageRecorder(t *testing.T, requestID string, input *anthropic.BetaMessageNewParams) *record.Recorder {
	t.Helper()
	recorder, err := record.New(record.Config{
		Enabled:       true,
		RequestID:     requestID,
		InputProtocol: protocol.TypeAnthropicBeta,
		Input:         input,
	})
	if err != nil {
		t.Fatal(err)
	}
	return recorder
}

func composeRecordedBetaToolLoop(t *testing.T, terminal protocolstage.Endpoint) protocolstage.Endpoint {
	t.Helper()
	toolStage, err := NewAnthropicBetaStage(AnthropicBetaStageConfig{
		Tools: staticBetaStageTools{tools: []anthropic.BetaToolUnionParam{betaStageToolDefinition("lookup")}},
		Executor: &fakeBetaStageExecutor{results: map[string]ToolExecutionResult{
			"lookup": {Contents: coretool.TextToolResult("ok").Contents},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := protocolstage.Compose(terminal, toolStage)
	if err != nil {
		t.Fatal(err)
	}
	return endpoint
}

func assertBetaStageToolLoopRecord(t *testing.T, completed *record.RequestRecord, toolID, finalText string) {
	t.Helper()
	if completed == nil || completed.Outcome != record.OutcomeSucceeded {
		t.Fatalf("completed record = %#v", completed)
	}
	if len(completed.ProviderExchanges) != 2 {
		t.Fatalf("provider exchanges = %d, want 2", len(completed.ProviderExchanges))
	}
	for i, exchange := range completed.ProviderExchanges {
		if exchange.Sequence != i+1 || exchange.Attempt != 4 || exchange.Provider != "provider-a" || exchange.Protocol != protocol.TypeAnthropicBeta {
			t.Fatalf("exchange %d metadata = %#v", i, exchange)
		}
		if exchange.Outcome != record.OutcomeSucceeded || exchange.Response == nil {
			t.Fatalf("exchange %d result = %#v", i, exchange)
		}
	}
	firstResponse := string(completed.ProviderExchanges[0].Response.Body)
	if !strings.Contains(firstResponse, toolID) || !strings.Contains(firstResponse, "lookup") {
		t.Fatalf("first provider response = %s", firstResponse)
	}
	secondRequest := string(completed.ProviderExchanges[1].Request.Body)
	if !strings.Contains(secondRequest, toolID) || !strings.Contains(secondRequest, "tool_result") {
		t.Fatalf("second provider request = %s", secondRequest)
	}
	secondResponse := string(completed.ProviderExchanges[1].Response.Body)
	if !strings.Contains(secondResponse, finalText) {
		t.Fatalf("second provider response = %s", secondResponse)
	}
	if completed.FinalResponse == nil || completed.FinalResponse.Protocol != protocol.TypeAnthropicBeta || !strings.Contains(string(completed.FinalResponse.Body), finalText) {
		t.Fatalf("final response = %#v", completed.FinalResponse)
	}
	if strings.Contains(string(completed.InputRequest.Body), "lookup") {
		t.Fatalf("input request was captured after Stage mutation: %s", completed.InputRequest.Body)
	}
}
