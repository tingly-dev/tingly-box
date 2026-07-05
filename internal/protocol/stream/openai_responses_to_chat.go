package stream

import (
	"context"
	"errors"

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

	conv := newResponsesToChatConverter(stream, responseModel, hc.DisableStreamUsage)

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
