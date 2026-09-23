package client

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func imageStream(events ...string) *ssestream.Stream[responses.ResponseStreamEventUnion] {
	var body strings.Builder
	for _, e := range events {
		body.WriteString("data: " + e + "\n\n")
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body.String())),
	}
	return ssestream.NewStream[responses.ResponseStreamEventUnion](ssestream.NewDecoder(resp), nil)
}

const (
	partialA  = `{"type":"response.image_generation_call.partial_image","item_id":"ig_1","output_index":0,"partial_image_index":0,"partial_image_b64":"AAAA","sequence_number":1}`
	partialB  = `{"type":"response.image_generation_call.partial_image","item_id":"ig_1","output_index":0,"partial_image_index":1,"partial_image_b64":"BBBB","sequence_number":2}`
	doneFinal = `{"type":"response.output_item.done","output_index":0,"sequence_number":3,"item":{"type":"image_generation_call","id":"ig_1","status":"generating","result":"FINAL"}}`
)

func TestParseImageGenerationStream_FinalResultWinsOverPartials(t *testing.T) {
	c := &CodexClient{}
	resp, err := c.parseImageGenerationStream(context.Background(), imageStream(partialA, partialB, doneFinal))
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)
	// Partials are standalone previews, never concatenated; the done item's
	// result is the image even when its status still reads "generating".
	assert.Equal(t, "FINAL", resp.Data[0].B64JSON)
}

func TestParseImageGenerationStream_LastPartialWithoutFinal(t *testing.T) {
	c := &CodexClient{}
	resp, err := c.parseImageGenerationStream(context.Background(), imageStream(partialA, partialB))
	require.NoError(t, err)
	assert.Equal(t, "BBBB", resp.Data[0].B64JSON)
}

func TestParseImageGenerationStream_NoImage(t *testing.T) {
	c := &CodexClient{}
	_, err := c.parseImageGenerationStream(context.Background(), imageStream(`{"type":"response.completed","sequence_number":1}`))
	assert.Error(t, err)
}

// When the upstream declines to draw, the stream says why in its own events.
// That reason must be the error, not a bare "no image data".
func TestParseImageGenerationStream_NoImageCarriesUpstreamReason(t *testing.T) {
	cases := map[string]struct {
		events []string
		want   string
	}{
		"response.failed": {
			events: []string{`{"type":"response.failed","sequence_number":1,"response":{"id":"resp_1","status":"failed","error":{"code":"image_content_policy_violation","message":"Your request was rejected by the safety system."}}}`},
			want:   "image_content_policy_violation: Your request was rejected by the safety system.",
		},
		"error event": {
			events: []string{`{"type":"error","sequence_number":1,"code":"moderation_blocked","message":"Blocked by moderation.","param":null}`},
			want:   "moderation_blocked: Blocked by moderation.",
		},
		"model refused in text": {
			events: []string{
				`{"type":"response.output_item.done","output_index":0,"sequence_number":1,"item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"I can't create that image.","annotations":[]}]}}`,
				`{"type":"response.completed","sequence_number":2}`,
			},
			want: "I can't create that image.",
		},
		"refusal part": {
			events: []string{`{"type":"response.output_item.done","output_index":0,"sequence_number":1,"item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"refusal","refusal":"Not allowed."}]}}`},
			want:   "Not allowed.",
		},
		"failed image call": {
			events: []string{`{"type":"response.output_item.done","output_index":0,"sequence_number":1,"item":{"type":"image_generation_call","id":"ig_9","status":"failed"}}`},
			want:   "image_generation_call failed",
		},
		"incomplete": {
			events: []string{`{"type":"response.incomplete","sequence_number":1,"response":{"id":"resp_1","status":"incomplete","incomplete_details":{"reason":"content_filter"}}}`},
			want:   "incomplete: content_filter",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := &CodexClient{}
			_, err := c.parseImageGenerationStream(context.Background(), imageStream(tc.events...))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}
