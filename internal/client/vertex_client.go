package client

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"sync"

	anthropicOption "github.com/anthropics/anthropic-sdk-go/v2/option"
	anthropicVertex "github.com/anthropics/anthropic-sdk-go/v2/vertex"

	gauth "cloud.google.com/go/auth"
	gcreds "cloud.google.com/go/auth/credentials"
	"cloud.google.com/go/auth/httptransport"
	"cloud.google.com/go/auth/oauth2adapt"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/genai"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// vertexScope is the OAuth2 scope required to call Vertex AI with a service
// account, shared by the Anthropic-on-Vertex and Gemini-on-Vertex paths.
const vertexScope = "https://www.googleapis.com/auth/cloud-platform"

// Credential cache, keyed by sha256 of the service-account JSON. Clients are
// rebuilt per request (see pool.go), but the parsed credentials — whose token
// source caches the minted OAuth access token until expiry — must be reused
// across requests. Without this every request re-parses the SA key (RSA
// decode) and mints a fresh token via a blocking round-trip to Google's token
// endpoint. A changed SA JSON hashes to a new key, so updates take effect
// immediately; stale entries are bounded by the number of distinct SA keys.
//
// One entry serves both Vertex paths: the genai credentials are parsed once
// and the oauth2 form (for the anthropic vertex adapter) is derived from them
// via oauth2adapt, so both share a single cached token per SA key.
var vertexCredsCache sync.Map // [32]byte -> vertexCreds

type vertexCreds struct {
	genai  *gauth.Credentials
	oauth2 *google.Credentials
}

// validateCloudBundle checks that a multi-field provider carries a credential
// bundle satisfying its auth type's schema. Shared prologue for every cloud
// client constructor so none can forget the nil guard.
func validateCloudBundle(provider *typ.Provider) error {
	if provider.Credential == nil {
		return fmt.Errorf("provider %q: missing credential bundle for auth type %s", provider.Name, provider.AuthType)
	}
	return ai.ValidateCredential(provider.AuthType, provider.Credential.Fields)
}

// NewVertexAnthropicClient builds an Anthropic client that targets Claude on GCP
// Vertex AI. The anthropic-sdk-go/vertex adapter loads a service-account OAuth2
// token source and rewrites /v1/messages to the Vertex publisher endpoint; this
// constructor supplies the credentials from the stored bundle.
func NewVertexAnthropicClient(provider *typ.Provider, model string, sessionID typ.SessionID) (*AnthropicClient, error) {
	opts, err := vertexAnthropicOptions(context.Background(), provider, model, sessionID)
	if err != nil {
		return nil, err
	}
	return NewAnthropicClient(provider, model, sessionID, opts...)
}

// vertexAnthropicOptions resolves the Vertex adapter RequestOptions from a
// provider's GCP service-account bundle.
//
// vertex.WithCredentials replaces the whole HTTP client with one built by
// google.golang.org/api/transport — its way of installing the token-refreshing
// auth transport — which would discard our transport chain: provider.ProxyURL
// would be ignored and env proxies inherited, both against this project's
// proxy semantics. Auth and rewriting are orthogonal there (URL/body rewriting
// is middleware, auth is just a bearer from the token source), so we re-apply
// an HTTP client of our own AFTER the adapter option: the same transport chain
// generic Anthropic providers get, wrapped in oauth2.Transport for the token.
func vertexAnthropicOptions(ctx context.Context, provider *typ.Provider, model string, sessionID typ.SessionID) ([]anthropicOption.RequestOption, error) {
	if err := validateCloudBundle(provider); err != nil {
		return nil, err
	}
	saJSON := provider.Credential.Field(ai.CredFieldGCPServiceAccountJSON)
	creds, err := cachedGoogleCredentials(ctx, saJSON)
	if err != nil {
		return nil, fmt.Errorf("provider %q: invalid GCP service account JSON: %w", provider.Name, err)
	}
	location := provider.Credential.Field(ai.CredFieldGCPLocation)
	project := provider.Credential.Field(ai.CredFieldGCPProjectID)
	return []anthropicOption.RequestOption{
		anthropicVertex.WithCredentials(ctx, location, project, creds),
		anthropicOption.WithHTTPClient(vertexAuthedHTTPClient(provider, model, sessionID, creds)),
	}, nil
}

// vertexAuthedHTTPClient layers the shared SA OAuth token source over the
// standard provider transport chain (proxy_url, UA, logging).
func vertexAuthedHTTPClient(provider *typ.Provider, model string, sessionID typ.SessionID, creds *google.Credentials) *http.Client {
	return &http.Client{
		Transport: &oauth2.Transport{
			Source: creds.TokenSource,
			Base:   anthropicTransport(provider, model, sessionID),
		},
	}
}

// applyVertexToGenaiConfig mutates cfg so the go-genai client targets Gemini on
// Vertex AI using the provider's service-account credentials. It is invoked from
// NewGoogleClient for gcp_sa providers; go-genai has no request-option seam, so
// the Vertex wiring lives on the config directly.
func applyVertexToGenaiConfig(ctx context.Context, provider *typ.Provider, cfg *genai.ClientConfig) error {
	if err := validateCloudBundle(provider); err != nil {
		return err
	}
	saJSON := provider.Credential.Field(ai.CredFieldGCPServiceAccountJSON)
	creds, err := cachedGenaiCredentials(saJSON)
	if err != nil {
		return fmt.Errorf("provider %q: invalid GCP service account JSON: %w", provider.Name, err)
	}

	// APIKey must be empty on the Vertex backend; clear whatever the generic
	// path set from GetAccessToken() (which is "" for gcp_sa anyway).
	cfg.APIKey = ""
	// Leave BaseURL empty: genai derives the correct Vertex host from
	// Backend/Location — including the special "global"
	// (aiplatform.googleapis.com) and multi-regional "us"/"eu"
	// (aiplatform.<loc>.rep.googleapis.com) hosts — but only when
	// HTTPOptions.BaseURL is unset; a stored APIBase would override it with a
	// possibly wrong host.
	cfg.HTTPOptions.BaseURL = ""
	cfg.Backend = genai.BackendVertexAI
	cfg.Project = provider.Credential.Field(ai.CredFieldGCPProjectID)
	cfg.Location = provider.Credential.Field(ai.CredFieldGCPLocation)
	cfg.Credentials = creds

	// genai only auto-installs auth when it constructs the HTTP client itself
	// (ClientConfig.HTTPClient == nil). We always pass our own client (proxy +
	// logging transport), so the OAuth2 bearer must be layered on explicitly —
	// otherwise Vertex requests go out unauthenticated. Fail closed rather than
	// silently sending unsigned requests if a future caller passes no client.
	if cfg.HTTPClient == nil {
		return fmt.Errorf("provider %q: Vertex genai config requires an HTTP client to attach auth", provider.Name)
	}
	if err := httptransport.AddAuthorizationMiddleware(cfg.HTTPClient, creds); err != nil {
		return fmt.Errorf("provider %q: failed to attach Vertex auth: %w", provider.Name, err)
	}
	return nil
}

// cachedVertexCreds parses (once) and caches the credentials for a
// service-account JSON in both the genai and oauth2 forms.
func cachedVertexCreds(saJSON string) (vertexCreds, error) {
	key := sha256.Sum256([]byte(saJSON))
	if v, ok := vertexCredsCache.Load(key); ok {
		return v.(vertexCreds), nil
	}
	creds, err := gcreds.DetectDefault(&gcreds.DetectOptions{
		CredentialsJSON: []byte(saJSON),
		Scopes:          []string{vertexScope},
	})
	if err != nil {
		return vertexCreds{}, err
	}
	entry := vertexCreds{
		genai:  creds,
		oauth2: oauth2adapt.Oauth2CredentialsFromAuthCredentials(creds),
	}
	v, _ := vertexCredsCache.LoadOrStore(key, entry)
	return v.(vertexCreds), nil
}

// cachedGoogleCredentials returns the oauth2 form used by the anthropic
// vertex adapter.
func cachedGoogleCredentials(_ context.Context, saJSON string) (*google.Credentials, error) {
	creds, err := cachedVertexCreds(saJSON)
	return creds.oauth2, err
}

// cachedGenaiCredentials returns the cloud.google.com/go/auth form used by
// the go-genai Vertex backend.
func cachedGenaiCredentials(saJSON string) (*gauth.Credentials, error) {
	creds, err := cachedVertexCreds(saJSON)
	return creds.genai, err
}
