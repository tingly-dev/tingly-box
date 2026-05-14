package imagegen

import (
	"errors"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestDetectVendor(t *testing.T) {
	cases := []struct {
		name  string
		base  string
		style protocol.APIStyle
		want  Vendor
	}{
		{"openai", "https://api.openai.com/v1", protocol.APIStyleOpenAI, VendorOpenAICompat},
		{"xai", "https://api.x.ai/v1/", protocol.APIStyleOpenAI, VendorOpenAICompat},
		{"volcengine", "https://ark.cn-beijing.volces.com/api/v3", protocol.APIStyleOpenAI, VendorOpenAICompat},
		{"zhipu", "https://open.bigmodel.cn/api/paas/v4", protocol.APIStyleOpenAI, VendorOpenAICompat},
		{"dashscope-cn", "https://dashscope.aliyuncs.com/compatible-mode/v1", protocol.APIStyleOpenAI, VendorDashScope},
		{"dashscope-intl", "https://dashscope-intl.aliyuncs.com/compatible-mode/v1", protocol.APIStyleOpenAI, VendorDashScope},
		{"minimax-com", "https://api.minimaxi.com/v1", protocol.APIStyleOpenAI, VendorMinimax},
		{"minimax-io", "https://api.minimax.io/v1", protocol.APIStyleOpenAI, VendorMinimax},
		{"codex-base", protocol.CodexAPIBase, protocol.APIStyleOpenAI, VendorCodex},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &typ.Provider{Name: tc.name, APIBase: tc.base, APIStyle: tc.style}
			if got := DetectVendor(p); got != tc.want {
				t.Fatalf("DetectVendor(%s) = %s, want %s", tc.base, got, tc.want)
			}
		})
	}
}

func TestDetectVendorNil(t *testing.T) {
	if got := DetectVendor(nil); got != VendorUnknown {
		t.Fatalf("DetectVendor(nil) = %s, want %s", got, VendorUnknown)
	}
}

func TestRequestRoundTrip(t *testing.T) {
	p := &openai.ImageGenerateParams{
		Model:          openai.ImageModel("gpt-image-1"),
		Prompt:         "a red fox",
		N:              param.NewOpt(int64(2)),
		Size:           openai.ImageGenerateParamsSize("1024x1024"),
		Quality:        openai.ImageGenerateParamsQuality("high"),
		ResponseFormat: openai.ImageGenerateParamsResponseFormat("b64_json"),
	}
	req := RequestFromOpenAI(p)
	if req.Model != "gpt-image-1" || req.Prompt != "a red fox" || req.N != 2 {
		t.Fatalf("unexpected request: %+v", req)
	}
	if req.Size != "1024x1024" || req.Quality != "high" || req.ResponseFormat != "b64_json" {
		t.Fatalf("unexpected request fields: %+v", req)
	}

	out := req.ToOpenAIParams()
	if string(out.Model) != "gpt-image-1" || !out.N.Valid() || out.N.Value != 2 {
		t.Fatalf("ToOpenAIParams lost data: %+v", out)
	}
}

func TestResponseToOpenAI(t *testing.T) {
	r := &Response{
		Created: 123,
		Model:   "gpt-image-1",
		Data:    []Image{{URL: "https://example.com/a.png"}, {B64JSON: "abc"}},
		Usage:   Usage{InputTokens: 10, OutputTokens: 20, TotalTokens: 30},
	}
	out := r.ToOpenAI()
	if out.Created != 123 || len(out.Data) != 2 {
		t.Fatalf("unexpected response: %+v", out)
	}
	if out.Data[0].URL != "https://example.com/a.png" || out.Data[1].B64JSON != "abc" {
		t.Fatalf("unexpected data: %+v", out.Data)
	}
	if out.Usage.TotalTokens != 30 {
		t.Fatalf("unexpected usage: %+v", out.Usage)
	}
}

func TestMinimaxAspectRatio(t *testing.T) {
	cases := []struct {
		size string
		want string
	}{
		{"1024x1024", "1:1"},
		{"1792x1024", "16:9"},
		{"1024x1792", "9:16"},
		{"1234x5678", ""}, // unsupported ratio -> upstream default
		{"", ""},
	}
	for _, tc := range cases {
		got := minimaxAspectRatio(&Request{Size: tc.size})
		if got != tc.want {
			t.Fatalf("minimaxAspectRatio(%q) = %q, want %q", tc.size, got, tc.want)
		}
	}
	// Explicit override wins.
	got := minimaxAspectRatio(&Request{Size: "1024x1024", Extra: map[string]any{"aspect_ratio": "21:9"}})
	if got != "21:9" {
		t.Fatalf("explicit aspect_ratio override = %q, want 21:9", got)
	}
}

func TestDashScopeSize(t *testing.T) {
	if got := dashscopeSize("1024x1024"); got != "1024*1024" {
		t.Fatalf("dashscopeSize = %q, want 1024*1024", got)
	}
	if got := dashscopeSize(""); got != "" {
		t.Fatalf("dashscopeSize(empty) = %q, want empty", got)
	}
}

func TestNewOpenAICompatReturnsUnsupported(t *testing.T) {
	// OpenAI-compatible and Codex providers are not served by imagegen.New —
	// client.OpenAIClient / client.CodexClient handle them natively.
	for _, base := range []string{"https://api.openai.com/v1", protocol.CodexAPIBase} {
		p := &typ.Provider{Name: "compat", APIBase: base, APIStyle: protocol.APIStyleOpenAI}
		if _, err := New(p, "dall-e-3"); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("New(%s) err = %v, want ErrUnsupported", base, err)
		}
	}
}
