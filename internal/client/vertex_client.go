package client

import (
	"context"
	"fmt"

	anthropicOption "github.com/anthropics/anthropic-sdk-go/option"
	anthropicVertex "github.com/anthropics/anthropic-sdk-go/vertex"

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
	if err := validateVertexBundle(provider); err != nil {
		return nil, err
	}
	creds, err := google.CredentialsFromJSON(ctx,
		[]byte(provider.Credential.Field(ai.CredFieldGCPServiceAccountJSON)), vertexScope)
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
	if err := validateVertexBundle(provider); err != nil {
		return err
	}
	creds, err := gcreds.DetectDefault(&gcreds.DetectOptions{
		CredentialsJSON: []byte(provider.Credential.Field(ai.CredFieldGCPServiceAccountJSON)),
		Scopes:          []string{vertexScope},
	})
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
	// otherwise Vertex requests go out unauthenticated.
	if cfg.HTTPClient != nil {
		if err := httptransport.AddAuthorizationMiddleware(cfg.HTTPClient, creds); err != nil {
			return fmt.Errorf("provider %q: failed to attach Vertex auth: %w", provider.Name, err)
		}
	}
	return nil
}

func validateVertexBundle(provider *typ.Provider) error {
	if provider.Credential == nil {
		return fmt.Errorf("provider %q: missing credential bundle for auth type %s", provider.Name, provider.AuthType)
	}
	return ai.ValidateCredential(provider.AuthType, provider.Credential.Fields)
}
