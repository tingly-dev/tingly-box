package command

import (
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
	oauth2 "github.com/tingly-dev/tingly-box/ai/oauth"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

func TestGetProviderConfig_AntigravityCodeAssist(t *testing.T) {
	config, err := getProviderConfig("antigravity")
	if err != nil {
		t.Fatal(err)
	}
	if config.APIBase != "https://cloudcode-pa.googleapis.com" || config.APIStyle != "google" {
		t.Fatalf("Antigravity must use Google Code Assist: %+v", config)
	}
}

func TestCreateProviderFromToken_CodeAssist(t *testing.T) {
	for _, issuer := range []string{"antigravity", "gemini"} {
		t.Run(issuer, func(t *testing.T) {
			am := newTestAppManager(t)
			config, err := getProviderConfig(issuer)
			if err != nil {
				t.Fatal(err)
			}
			token := &oauth2.Token{
				AccessToken:  "test-access-token",
				RefreshToken: "test-refresh-token",
				Metadata:     map[string]any{"project_id": "test-project"},
			}
			withSilencedStdout(t, func() {
				err = createProviderFromToken(am.AppConfig(), config, "test-"+issuer, token)
			})
			if err != nil {
				t.Fatal(err)
			}
			provider, err := am.AppConfig().GetProviderByName("test-" + issuer)
			if err != nil {
				t.Fatal(err)
			}
			if provider.APIStyle != protocol.APIStyleGoogle || provider.APIBase != "https://cloudcode-pa.googleapis.com" {
				t.Fatalf("saved incompatible Code Assist endpoint: style=%s, base=%s", provider.APIStyle, provider.APIBase)
			}
			if provider.OAuthDetail.GetIssuer() != ai.Issuer(issuer) || provider.OAuthDetail.GetExtraFieldString("project_id") != "test-project" {
				t.Fatal("saved provider lost OAuth issuer or project")
			}
		})
	}
}

// TestOAuthCmdKong_NoProvider_NoTTY_ClearError guards the no-TTY crash: with
// no provider argument, Run used to fall straight into runInteractiveMode's
// bufio prompt, which failed with a bare "failed to read input: EOF" once
// stdin wasn't a TTY (e.g. piped input, CI). It must fail with a clear,
// actionable error instead.
func TestOAuthCmdKong_NoProvider_NoTTY_ClearError(t *testing.T) {
	withNonTTYStdin(t)

	am := newTestAppManager(t)
	err := (&OAuthCmdKong{}).Run(am)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "no TTY") {
		t.Errorf("error = %q, want it to mention the missing TTY", err.Error())
	}
}
