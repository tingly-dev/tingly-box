package stream

import (
	"context"
	"errors"
	"io"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

// HandleAnthropicBetaToOpenAIResponsesStream converts Anthropic Beta streaming
// to Responses API format using the chain pipeline architecture.
func HandleAnthropicBetaToOpenAIResponsesStream(
	hc *protocol.HandleContext,
	stream *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion],
	responseModel string,
) (*protocol.TokenUsage, error) {
	c := hc.GinContext
	defer func() {
		if stream != nil {
			stream.Close()
		}
	}()

	conv := newAnthropicBetaToResponsesConverter(stream, responseModel, hc.Namespaces)

	usage, err := RunConverter(hc, conv, responsesSSEWriter(c))

	if err != nil {
		if !c.Writer.Written() && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
			logrus.WithContext(c.Request.Context()).Errorf("Anthropic to Responses pre-stream error: %v", err)
			SendStreamingError(c, err)
			return protocol.ZeroTokenUsage(), err
		}
		if errors.Is(err, context.Canceled) {
			logrus.WithContext(c.Request.Context()).Debug("Anthropic to Responses stream canceled by client")
			return conv.Usage(), nil
		}
		if errors.Is(err, io.EOF) {
			logrus.WithContext(c.Request.Context()).Info("Anthropic stream ended normally (EOF)")
			OpenAISSEDone(c)
			return conv.Usage(), nil
		}
		logrus.WithContext(c.Request.Context()).Errorf("Anthropic stream error: %v", err)
		sendResponsesErrorEvent(c, conv.nextSeq(), "stream_failed", protocol.UpstreamMessage(err))
		return conv.Usage(), err
	}

	OpenAISSEDone(c)
	return usage, nil
}

// sendResponsesEvent sends a single Responses API event as SSE.
func sendResponsesEvent(c *gin.Context, event any, _ interface{ Flush() }) {
	if e, ok := event.(wire.ResponsesEvent); ok {
		OpenAIResponsesEvent(c, e.EventType(), event)
	} else {
		OpenAISSE(c, event)
	}
}

// sendResponsesErrorEvent sends an error event in Responses API format.
func sendResponsesErrorEvent(c *gin.Context, seq int, code, message string) {
	errorEvent := wire.NewResponsesStreamErrorEvent(int64(seq), code, message)
	OpenAIResponsesEvent(c, errorEvent.EventType(), errorEvent)
}
