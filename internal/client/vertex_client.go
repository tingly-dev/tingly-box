package client

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"

	anthropicOption "github.com/anthropics/anthropic-sdk-go/option"
	anthropicVertex "github.com/anthropics/anthropic-sdk-go/vertex"

	gauth "cloud.google.com/go/auth"
	gcreds "cloud.google.com/go/auth/credentials"
	"cloud.google.com/go/auth/httptransport"
	"golang.org/x/oauth2/google"
	"google.golang.org/genai"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// vertexScope is the OAuth2 scope required to call Vertex AI with a service
// account, shared by the Anthropic-on-Vertex and Gemini-on-Vertex paths.
const vertexScope = "https://www.googleapis.com/auth/cloud-platform"

// Credential caches, keyed by sha256 of the service-account JSON. Clients are
// rebuilt per request (see pool.go), but the parsed credentials — whose token
// source caches the minted OAuth access token until expiry — must be reused
// across requests. Without this every request re-parses the SA key (RSA
// decode) and mints a fresh token via a blocking round-trip to Google's token
// endpoint. A changed SA JSON hashes to a new key, so updates take effect
// immediately; stale entries are bounded by the number of distinct SA keys.
var (
	vertexGoogleCredsCache sync.Map // [32]byte -> *google.Credentials
	vertexGenaiCredsCache  sync.Map // [32]byte -> *auth.Credentials
)

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
//
// Note: the vertex adapter installs its own HTTP client (google's auth
// transport), so provider.ProxyURL is not honored on this path today — a
// documented v1 limitation.
func NewVertexAnthropicClient(provider *typ.Provider, model string, sessionID typ.SessionID) (*AnthropicClient, error) {
	opt, err := vertexAnthropicOption(context.Background(), provider)
	if err != nil {
		return nil, err
	}
	return NewAnthropicClient(provider, model, sessionID, opt)
}

// vertexAnthropicOption resolves the Vertex adapter RequestOption from a
// provider's GCP service-account bundle.
func vertexAnthropicOption(ctx context.Context, provider *typ.Provider) (anthropicOption.RequestOption, error) {
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
	return anthropicVertex.WithCredentials(ctx, location, project, creds), nil
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

// cachedGoogleCredentials parses (once) and caches golang.org/x/oauth2/google
// credentials for a service-account JSON. The credential's ReuseTokenSource
// then serves cached access tokens across requests.
func cachedGoogleCredentials(ctx context.Context, saJSON string) (*google.Credentials, error) {
	key := sha256.Sum256([]byte(saJSON))
	if v, ok := vertexGoogleCredsCache.Load(key); ok {
		return v.(*google.Credentials), nil
	}
	creds, err := google.CredentialsFromJSON(ctx, []byte(saJSON), vertexScope)
	if err != nil {
		return nil, err
	}
	v, _ := vertexGoogleCredsCache.LoadOrStore(key, creds)
	return v.(*google.Credentials), nil
}

// cachedGenaiCredentials is the cloud.google.com/go/auth counterpart used by
// the go-genai Vertex backend.
func cachedGenaiCredentials(saJSON string) (*gauth.Credentials, error) {
	key := sha256.Sum256([]byte(saJSON))
	if v, ok := vertexGenaiCredsCache.Load(key); ok {
		return v.(*gauth.Credentials), nil
	}
	creds, err := gcreds.DetectDefault(&gcreds.DetectOptions{
		CredentialsJSON: []byte(saJSON),
		Scopes:          []string{vertexScope},
	})
	if err != nil {
		return nil, err
	}
	v, _ := vertexGenaiCredsCache.LoadOrStore(key, creds)
	return v.(*gauth.Credentials), nil
}
