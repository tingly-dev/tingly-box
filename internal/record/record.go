// Package record defines request-scoped protocol recording primitives.
//
// It is intentionally independent of HTTP, Gin, routing, usage accounting,
// and persistence. Callers create a Recorder only when recording is explicitly
// enabled; the disabled path returns nil and performs no payload capture.
package record

import (
	"encoding/json"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// Outcome is the terminal result of a request or provider exchange.
type Outcome string

const (
	OutcomePending   Outcome = "pending"
	OutcomeSucceeded Outcome = "succeeded"
	OutcomeFailed    Outcome = "failed"
	OutcomeCancelled Outcome = "cancelled"
)

// Payload is one complete value at a stable protocol boundary.
type Payload struct {
	Protocol    protocol.APIType `json:"protocol"`
	ContentType string           `json:"content_type"`
	Body        json.RawMessage  `json:"body"`
}

// RequestRecord is the completed record for one incoming client request.
type RequestRecord struct {
	Timestamp         time.Time          `json:"timestamp"`
	RequestID         string             `json:"request_id"`
	SessionID         string             `json:"session_id,omitempty"`
	Scenario          string             `json:"scenario,omitempty"`
	InputRequest      Payload            `json:"input_request"`
	ProviderExchanges []ProviderExchange `json:"provider_exchanges,omitempty"`
	FinalResponse     *Payload           `json:"final_response,omitempty"`
	Outcome           Outcome            `json:"outcome"`
	Error             string             `json:"error,omitempty"`
	Duration          time.Duration      `json:"duration"`
}

// ProviderExchange records one actual invocation of a provider endpoint.
// Sequence is one-based and reflects invocation order within RequestRecord.
type ProviderExchange struct {
	Sequence  int              `json:"sequence"`
	Attempt   int              `json:"attempt"`
	Provider  string           `json:"provider,omitempty"`
	Model     string           `json:"model,omitempty"`
	Protocol  protocol.APIType `json:"protocol"`
	Request   Payload          `json:"provider_request"`
	Response  *Payload         `json:"provider_response,omitempty"`
	Outcome   Outcome          `json:"outcome"`
	Error     string           `json:"error,omitempty"`
	StartedAt time.Time        `json:"started_at"`
	Duration  time.Duration    `json:"duration"`
}

// ExchangeMetadata identifies one provider endpoint invocation.
type ExchangeMetadata struct {
	Attempt  int
	Provider string
	Model    string
	Protocol protocol.APIType
}

func clonePayload(src Payload) Payload {
	dst := src
	dst.Body = append(json.RawMessage(nil), src.Body...)
	return dst
}

func clonePayloadPointer(src *Payload) *Payload {
	if src == nil {
		return nil
	}
	dst := clonePayload(*src)
	return &dst
}

func cloneRequestRecord(src RequestRecord) RequestRecord {
	dst := src
	dst.InputRequest = clonePayload(src.InputRequest)
	dst.FinalResponse = clonePayloadPointer(src.FinalResponse)
	dst.ProviderExchanges = make([]ProviderExchange, len(src.ProviderExchanges))
	for i := range src.ProviderExchanges {
		dst.ProviderExchanges[i] = src.ProviderExchanges[i]
		dst.ProviderExchanges[i].Request = clonePayload(src.ProviderExchanges[i].Request)
		dst.ProviderExchanges[i].Response = clonePayloadPointer(src.ProviderExchanges[i].Response)
	}
	return dst
}
