package stream

import (
	"context"
	"errors"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

// HandleOpenAIChatToResponsesStream converts OpenAI Chat Completions streaming
// to Responses API format using the chain pipeline architecture.
func HandleOpenAIChatToResponsesStream(hc *protocol.HandleContext, stream *openaistream.Stream[openai.ChatCompletionChunk], responseModel string) (*protocol.TokenUsage, error) {
	c := hc.GinContext
	defer func() {
		if stream != nil {
			stream.Close()
		}
	}()

	conv := NewChatToResponsesConverter(stream, responseModel)

	usage, err := RunConverter(hc, conv, responsesSSEWriter(c))

	if err != nil {
		if errors.Is(err, context.Canceled) {
			logrus.WithContext(c.Request.Context()).Debug("Chat to Responses stream canceled by client")
			return conv.Usage(), nil
		}

		logrus.WithContext(c.Request.Context()).Errorf("Chat to Responses stream error: %v", err)

		if !c.Writer.Written() {
			SendStreamingError(c, err)
			return conv.Usage(), err
		}

		errorEvent := wire.ResponsesStreamErrorEvent{
			Type:           "error",
			SequenceNumber: conv.nextSeq(),
			Error: wire.ResponsesStreamErrorBody{
				Message: err.Error(),
				Type:    "stream_error",
			},
		}
		OpenAIResponsesEvent(c, errorEvent.EventType(), errorEvent)
		return conv.Usage(), err
	}

	OpenAISSEDone(c)
	return usage, nil
}

// responsesSSEWriter returns a handleFunc that writes Responses API wire
// events as SSE to the gin context.
func responsesSSEWriter(c *gin.Context) func(event interface{}) error {
	return func(event interface{}) error {
		evt, ok := event.(wire.ResponsesEvent)
		if !ok {
			return fmt.Errorf("unexpected event type %T", event)
		}
		OpenAIResponsesEvent(c, evt.EventType(), evt)
		return nil
	}
}
