package server

import (
	"testing"
)

func TestParseVisionProxyService(t *testing.T) {
	cases := []struct {
		name string
		ext  map[string]interface{}
		want *struct{ provider, model string }
	}{
		{
			name: "nil extensions",
			ext:  nil,
			want: nil,
		},
		{
			name: "missing key",
			ext:  map[string]interface{}{"other": "value"},
			want: nil,
		},
		{
			name: "wrong shape",
			ext:  map[string]interface{}{"vision_proxy_service": "not-an-object"},
			want: nil,
		},
		{
			name: "missing provider",
			ext: map[string]interface{}{
				"vision_proxy_service": map[string]interface{}{
					"model": "claude-3-5-sonnet",
				},
			},
			want: nil,
		},
		{
			name: "missing model",
			ext: map[string]interface{}{
				"vision_proxy_service": map[string]interface{}{
					"provider": "p-uuid",
				},
			},
			want: nil,
		},
		{
			name: "empty strings",
			ext: map[string]interface{}{
				"vision_proxy_service": map[string]interface{}{
					"provider": "",
					"model":    "",
				},
			},
			want: nil,
		},
		{
			name: "valid",
			ext: map[string]interface{}{
				"vision_proxy_service": map[string]interface{}{
					"provider": "p-uuid",
					"model":    "claude-3-5-sonnet",
				},
			},
			want: &struct{ provider, model string }{provider: "p-uuid", model: "claude-3-5-sonnet"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseVisionProxyService(tc.ext)
			if tc.want == nil {
				if got != nil {
					t.Fatalf("want nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("want non-nil, got nil")
			}
			if got.Provider != tc.want.provider || got.Model != tc.want.model {
				t.Fatalf("want {%s,%s}, got {%s,%s}", tc.want.provider, tc.want.model, got.Provider, got.Model)
			}
			if !got.Active {
				t.Fatal("parsed service should be active so VisionProxyProcessor.pickUsableService accepts it")
			}
		})
	}
}
