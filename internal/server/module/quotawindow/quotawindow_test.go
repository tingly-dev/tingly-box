package quotawindow

import (
	"context"
	"errors"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/probe"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestTick(t *testing.T) {
	oauth := func(uuid string, models ...string) *typ.Provider {
		return &typ.Provider{UUID: uuid, Enabled: true, AuthType: typ.AuthTypeOAuth,
			OAuthDetail: &typ.OAuthDetail{Issuer: "claude_code", AccessToken: "t"}, Models: models}
	}
	disabled := oauth("disabled", "m")
	disabled.Enabled = false
	apiKey := &typ.Provider{UUID: "key", Enabled: true, AuthType: typ.AuthTypeAPIKey, Token: "x", Models: []string{"m"}}

	var got []string
	send := func(_ context.Context, req *probe.E2ERequest) (*probe.E2EData, error) {
		got = append(got, req.ProviderUUID+"/"+req.Model)
		if !req.Direct || req.Stream == nil || *req.Stream {
			t.Errorf("expected direct non-streaming request, got %+v", req)
		}
		if req.Model == "broken" {
			return nil, errors.New("unavailable")
		}
		return &probe.E2EData{Success: true}, nil
	}

	Tick(context.Background(), send, []*typ.Provider{
		oauth("a", "broken", "works", "never"),
		disabled,
		apiKey,
		oauth("empty"),
	})

	want := []string{"a/broken", "a/works"}
	if len(got) != len(want) {
		t.Fatalf("requests = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("requests = %v, want %v", got, want)
		}
	}
}
