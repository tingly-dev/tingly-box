package anthropicbridge

import (
	"context"
	"errors"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2"

	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
)

// Provider SDK streams are lazy: a failure to open the stream (a 429, say)
// surfaces on the first read. The bridged stream must return that failure
// from its first Next, before any converted event, so the caller can still
// answer with the provider's status or fail over instead of committing to a
// stream it cannot finish.
func TestBridgedStreamSurfacesOpenFailureFirst(t *testing.T) {
	t.Parallel()
	openFailure := errors.New("429 Too Many Requests")
	for _, bridge := range []stage.Bridge{
		NewBetaToOpenAIChat(ChatOptions{}),
		NewBetaToOpenAIResponses(ResponsesOptions{}),
	} {
		bridge := bridge
		t.Run(string(bridge.Target()), func(t *testing.T) {
			t.Parallel()
			target := &memoryStream{nextErr: openFailure}
			terminal := &memoryEndpoint{
				api: bridge.Target(),
				stream: func(context.Context, stage.Call) (stage.EventStream, error) {
					return target, nil
				},
			}
			request := &anthropic.BetaMessageNewParams{
				Model:     "client-model",
				MaxTokens: 32,
				Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hi"))},
			}
			events, err := mustAdapt(t, terminal, bridge).Stream(context.Background(), stage.Call{Request: request})
			if err != nil {
				t.Fatalf("Stream() error = %v", err)
			}
			defer events.Close()
			event, err := events.Next(context.Background())
			if !errors.Is(err, openFailure) {
				t.Fatalf("first Next = %T %+v, %v; want the open failure before any event", event.Value, event.Value, err)
			}
		})
	}
}
