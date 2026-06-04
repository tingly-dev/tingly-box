package stream

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// HandleResponsesToOpenAIChatStream converts Responses API streaming to Chat
// Completions format using the chain pipeline architecture.
func HandleResponsesToOpenAIChatStream(
	hc *protocol.HandleContext,
	stream ResponsesStreamIter,
	responseModel string,
) (*protocol.TokenUsage, error) {
	c := hc.GinContext
	defer func() {
		if stream != nil {
			stream.Close()
		}
	}()

	conv := NewResponsesToChatConverter(stream, responseModel, hc.DisableStreamUsage)

	usage, err := RunConverter(hc, conv, openaiChatSSEWriter(c))

	if err != nil {
		if errors.Is(err, context.Canceled) {
			logrus.WithContext(c.Request.Context()).Debug("Responses to Chat stream canceled by client")
			return conv.Usage(), nil
		}
		logrus.WithContext(c.Request.Context()).Errorf("Responses to Chat stream error: %v", err)
		if !c.Writer.Written() {
			SendStreamingError(c, err)
		}
		return conv.Usage(), err
	}

	OpenAISSEDone(c)
	return usage, nil
}

// openaiChatSSEWriter returns a handleFunc that writes OpenAI Chat wire chunks
// (both normal chunks and error chunks) as SSE.
func openaiChatSSEWriter(c *gin.Context) func(event interface{}) error {
	return func(event interface{}) error {
		OpenAISSE(c, event)
		return nil
	}
}

// writeSSEChunk writes a single SSE chunk — kept for callers in other files.
func writeSSEChunk(c *gin.Context, _ interface{ Flush() }, chunk any) {
	OpenAISSE(c, chunk)
}
