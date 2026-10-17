package client

import (
	"net/http"
	"strings"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/oauth"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// workbuddyRoundTripper layers the WorkBuddy client identification on an
// inner transport. The Authorization Bearer is set by the OpenAI SDK (and by
// ListModels for /v3/config). The
// values come from ai.WorkBuddySiteFor, the same table the login uses, so
// login and inference always present the same client.
type workbuddyRoundTripper struct {
	http.RoundTripper
	site   ai.WorkBuddySite
	userID string
}

func newWorkBuddyRoundTripper(inner http.RoundTripper, provider *typ.Provider) *workbuddyRoundTripper {
	t := &workbuddyRoundTripper{RoundTripper: inner}
	if provider != nil && provider.OAuthDetail != nil {
		t.site, _ = ai.WorkBuddySiteFor(provider.OAuthDetail.GetIssuer())
		if uid, ok := provider.OAuthDetail.ExtraFields[oauth.WorkBuddyMetaUserID].(string); ok {
			t.userID = uid
		}
	}
	return t
}

func (t *workbuddyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// A RoundTripper must not modify the caller's request.
	req = req.Clone(req.Context())
	for k, v := range t.site.ClientHeaders() {
		req.Header.Set(k, v)
	}
	// The product config answers per client: the CLI agent's model list
	// only comes back to the CLI User-Agent.
	if strings.HasSuffix(req.URL.Path, "/v3/config") {
		req.Header.Set("User-Agent", ai.WorkBuddyConfigUserAgent)
	}
	if t.userID != "" {
		req.Header.Set("X-User-Id", t.userID)
	}
	return t.RoundTripper.RoundTrip(req)
}
