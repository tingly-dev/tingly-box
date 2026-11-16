package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesClientsCompactionState(t *testing.T) {
	for _, codex := range []bool{false, true} {
		t.Run(map[bool]string{false: "openai", true: "codex"}[codex], func(t *testing.T) {
			calls := 0
			transport := rtPtr(func(r *http.Request) (*http.Response, error) {
				calls++
				raw, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.Equal(t, "compaction", gjson.GetBytes(raw, "context_management.0.type").String())
				require.Equal(t, int64(12345), gjson.GetBytes(raw, "context_management.0.compact_threshold").Int())
				require.Equal(t, "native-prior", gjson.GetBytes(raw, "input.0.encrypted_content").String())
				require.Equal(t, "compaction_trigger", gjson.GetBytes(raw, "input.1.type").String())
				payload := `data: {"type":"response.compaction.compacting","sequence_number":1,"future_event":true}` + "\n\n" +
					`data: {"type":"response.output_item.done","sequence_number":2,"output_index":0,"item":{"type":"compaction","id":"cmp_out","encrypted_content":"native-next","future_item":7}}` + "\n\n" +
					`data: {"type":"response.completed","sequence_number":3,"response":{"id":"resp_out","status":"completed","output":[{"type":"compaction","id":"cmp_out","encrypted_content":"native-next","future_item":7}]}}` + "\n\n"
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(payload)), Request: r}, nil
			})
			var roundTrip http.RoundTripper = transport
			if codex {
				roundTrip = &codexRoundTripper{RoundTripper: transport}
			}
			base := &OpenAIClient{client: openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL("https://mock.test/backend-api/"), option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: roundTrip}))}
			var client OpenAIClientInterface = base
			if codex {
				client = &CodexClient{OpenAIClient: base}
			}
			var params responses.ResponseNewParams
			require.NoError(t, json.Unmarshal([]byte(`{"model":"provider","context_management":[{"type":"compaction","compact_threshold":12345}],"input":[{"type":"compaction","encrypted_content":"native-prior"},{"type":"compaction_trigger"}]}`), &params))
			stream := client.ResponsesNewStreaming(context.Background(), params)
			defer stream.Close()
			var types []string
			for stream.Next() {
				event := stream.Current()
				types = append(types, event.Type)
				if event.Type == "response.compaction.compacting" {
					require.True(t, gjson.Get(event.RawJSON(), "future_event").Bool())
				}
				if event.Type == "response.output_item.done" {
					require.Equal(t, "native-next", gjson.Get(event.RawJSON(), "item.encrypted_content").String())
					require.Equal(t, int64(7), gjson.Get(event.RawJSON(), "item.future_item").Int())
				}
			}
			require.NoError(t, stream.Err())
			require.Equal(t, []string{"response.compaction.compacting", "response.output_item.done", "response.completed"}, types)
			require.Equal(t, 1, calls)
		})
	}
}
