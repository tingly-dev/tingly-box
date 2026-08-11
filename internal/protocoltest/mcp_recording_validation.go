package protocoltest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	requestrecord "github.com/tingly-dev/tingly-box/internal/record"
)

func verifyMCPStagePersistedRecord(
	env *TestEnv,
	recordDir string,
	requestModel string,
	source protocol.APIType,
	target protocol.APIType,
	existingRecordIDs map[string]struct{},
) []AssertionError {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := env.ForceFlushRecordings(ctx); err != nil {
		return []AssertionError{{Assertion: "request_record_flush", Error: err.Error()}}
	}

	records, err := readPersistedRequestRecordArtifacts(recordDir)
	if err != nil {
		return []AssertionError{{Assertion: "request_record_read", Error: err.Error()}}
	}
	matched := make([]*requestrecord.RequestRecord, 0, 1)
	for _, record := range records {
		if record != nil && bytes.Contains(record.InputRequest.Body, []byte(requestModel)) {
			if _, existed := existingRecordIDs[record.RequestID]; existed {
				continue
			}
			matched = append(matched, record)
		}
	}
	if len(matched) != 1 {
		return []AssertionError{{
			Assertion: "request_record_identity",
			Error:     fmt.Sprintf("new persisted records for request model %q = %d, want 1", requestModel, len(matched)),
		}}
	}

	record := matched[0]
	contextBody, _ := json.Marshal(record)
	contextText := truncate(string(contextBody), 600)
	var result []AssertionError
	check := func(assertion string, condition bool, format string, args ...any) {
		if condition {
			return
		}
		result = append(result, AssertionError{
			Assertion: assertion,
			Error:     fmt.Sprintf(format, args...),
			Context:   contextText,
		})
	}

	check("request_record_outcome", record.Outcome == requestrecord.OutcomeSucceeded,
		"request outcome = %q, want %q", record.Outcome, requestrecord.OutcomeSucceeded)
	check("request_record_input_protocol", record.InputRequest.Protocol == source,
		"input protocol = %q, want %q", record.InputRequest.Protocol, source)
	check("request_record_input_original", bytes.Contains(record.InputRequest.Body, []byte("capital of France")),
		"input request does not contain the original user prompt")
	check("request_record_input_unmodified", !bytes.Contains(record.InputRequest.Body, []byte(matrixOwnedToolName)),
		"input request already contains injected server tool %q", matrixOwnedToolName)
	check("request_record_exchange_count", len(record.ProviderExchanges) == 2,
		"provider exchange count = %d, want 2", len(record.ProviderExchanges))

	providerProtocol := target
	if target == protocol.TypeAnthropicV1 {
		// V1 MCP requests are promoted into the Beta-native Tool Loop and the
		// provider boundary records that concrete protocol.
		providerProtocol = protocol.TypeAnthropicBeta
	}
	for index, exchange := range record.ProviderExchanges {
		check(fmt.Sprintf("request_record_exchange_%d_sequence", index+1), exchange.Sequence == index+1,
			"exchange %d sequence = %d, want %d", index+1, exchange.Sequence, index+1)
		check(fmt.Sprintf("request_record_exchange_%d_attempt", index+1), exchange.Attempt == 1,
			"exchange %d attempt = %d, want 1", index+1, exchange.Attempt)
		check(fmt.Sprintf("request_record_exchange_%d_protocol", index+1), exchange.Protocol == providerProtocol,
			"exchange %d protocol = %q, want %q", index+1, exchange.Protocol, providerProtocol)
		check(fmt.Sprintf("request_record_exchange_%d_request_protocol", index+1), exchange.Request.Protocol == providerProtocol,
			"exchange %d request protocol = %q, want %q", index+1, exchange.Request.Protocol, providerProtocol)
		check(fmt.Sprintf("request_record_exchange_%d_outcome", index+1), exchange.Outcome == requestrecord.OutcomeSucceeded,
			"exchange %d outcome = %q, want %q", index+1, exchange.Outcome, requestrecord.OutcomeSucceeded)
		check(fmt.Sprintf("request_record_exchange_%d_response", index+1), exchange.Response != nil,
			"exchange %d provider response is missing", index+1)
		if exchange.Response != nil {
			check(fmt.Sprintf("request_record_exchange_%d_response_protocol", index+1), exchange.Response.Protocol == providerProtocol,
				"exchange %d response protocol = %q, want %q", index+1, exchange.Response.Protocol, providerProtocol)
		}
	}

	if len(record.ProviderExchanges) == 2 {
		first := record.ProviderExchanges[0]
		second := record.ProviderExchanges[1]
		check("request_record_first_provider_tool_injected", bytes.Contains(first.Request.Body, []byte(matrixOwnedToolName)),
			"first provider request does not contain injected server tool %q", matrixOwnedToolName)
		if first.Response != nil {
			check("request_record_first_provider_tool_call", bytes.Contains(first.Response.Body, []byte(matrixOwnedToolName)),
				"first provider response does not contain the owned tool call")
		}
		check("request_record_second_provider_tool_result", bytes.Contains(second.Request.Body, []byte("echo-result")),
			"second provider request does not contain the local tool result")
		if second.Response != nil {
			check("request_record_second_provider_final", bytes.Contains(second.Response.Body, []byte("owned-tool-final")),
				"second provider response does not contain the final answer")
		}
	}

	check("request_record_final_response", record.FinalResponse != nil,
		"final client response is missing")
	if record.FinalResponse != nil {
		check("request_record_final_protocol", record.FinalResponse.Protocol == source,
			"final response protocol = %q, want %q", record.FinalResponse.Protocol, source)
		check("request_record_final_content", bytes.Contains(record.FinalResponse.Body, []byte("owned-tool-final")),
			"final response does not contain the client-visible answer")
	}
	return result
}

func persistedRequestRecordIDs(recordDir string) (map[string]struct{}, error) {
	records, err := readPersistedRequestRecordArtifacts(recordDir)
	if err != nil {
		return nil, err
	}
	ids := make(map[string]struct{}, len(records))
	for _, record := range records {
		if record != nil && record.RequestID != "" {
			ids[record.RequestID] = struct{}{}
		}
	}
	return ids, nil
}
