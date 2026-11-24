package protocolserver

import (
	"fmt"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/vision/imagegen"
)

func TestParseImageReasoningEffort(t *testing.T) {
	for _, effort := range []string{"", "auto", "low", "medium", "high", "future"} {
		t.Run(effort, func(t *testing.T) {
			want := effort
			if want == "" {
				want = "auto"
			}
			extension := ""
			if effort != "" {
				extension = fmt.Sprintf(`,"reasoning_effort":%q`, effort)
			}
			var generation openai.ImageGenerateParams
			err := parseImageGenerationRequest([]byte(`{"model":"image-rule","prompt":"a fox"`+extension+`}`), &generation)
			require.NoError(t, err)
			assert.Equal(t, want, imagegen.ReasoningEffort(&generation))
			assert.Equal(t, "image-rule", generation.Model, "parsing must not interpret routed model names")

			body := `{"model":"image-rule","prompt":"a fox","image":"` + editTestPNGBase64 + `"` + extension + `}`
			edit, err := parseImageEditRequest(newEditTestContext(t, "POST", "application/json", strings.NewReader(body)))
			require.NoError(t, err)
			assert.Equal(t, want, imagegen.ReasoningEffort(edit))

			fields := map[string]string{"model": "image-rule", "prompt": "a fox"}
			if effort != "" {
				fields["reasoning_effort"] = effort
			}
			multipartBody, contentType := buildMultipartBody(t, "image", 1, fields)
			edit, err = parseImageEditRequest(newEditTestContext(t, "POST", contentType, multipartBody))
			require.NoError(t, err)
			assert.Equal(t, want, imagegen.ReasoningEffort(edit))
		})
	}
}
