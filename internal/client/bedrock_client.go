package client

import (
	"fmt"

	anthropicBedrock "github.com/anthropics/anthropic-sdk-go/bedrock"
	anthropicOption "github.com/anthropics/anthropic-sdk-go/option"
	awsconfig "github.com/aws/aws-sdk-go-v2/aws"
	awscreds "github.com/aws/aws-sdk-go-v2/credentials"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// NewBedrockClient builds an Anthropic-compatible client that targets Amazon
// Bedrock. Bedrock speaks the Anthropic Messages API for Claude models but
// authenticates with AWS SigV4 (or a Bedrock bearer token) and lives at a
// region-specific host. All of the URL/body rewriting, request signing, and
// eventstream→SSE normalization is done by the anthropic-sdk-go/bedrock adapter;
// this constructor only translates the stored credential bundle into it and
// layers it on top of the generic Anthropic client (which still owns proxy,
// User-Agent, logging, and timeout behavior).
func NewBedrockClient(provider *typ.Provider, model string, sessionID typ.SessionID) (*AnthropicClient, error) {
	opt, err := bedrockOption(provider)
	if err != nil {
		return nil, err
	}
	return NewAnthropicClient(provider, model, sessionID, opt)
}

// bedrockOption resolves the Bedrock adapter RequestOption from a provider's
// AWS credential bundle.
func bedrockOption(provider *typ.Provider) (anthropicOption.RequestOption, error) {
	if provider.Credential == nil {
		return nil, fmt.Errorf("provider %q: missing credential bundle for auth type %s", provider.Name, provider.AuthType)
	}
	if err := ai.ValidateCredential(provider.AuthType, provider.Credential.Fields); err != nil {
		return nil, err
	}
	return anthropicBedrock.WithConfig(awsConfigFromBundle(provider.Credential)), nil
}

// awsConfigFromBundle builds an aws.Config from an AWS credential bundle. A
// Bedrock bearer token, when present, is preferred (the adapter then uses bearer
// auth); otherwise static access-key/secret[/session] credentials are used for
// SigV4 signing. Region drives the bedrock-runtime host.
func awsConfigFromBundle(bundle *typ.CredentialBundle) awsconfig.Config {
	cfg := awsconfig.Config{
		Region: bundle.Field(ai.CredFieldAWSRegion),
	}
	if bearer := bundle.Field(ai.CredFieldAWSBearerToken); bearer != "" {
		cfg.BearerAuthTokenProvider = anthropicBedrock.NewStaticBearerTokenProvider(bearer)
		return cfg
	}
	cfg.Credentials = awscreds.NewStaticCredentialsProvider(
		bundle.Field(ai.CredFieldAWSAccessKeyID),
		bundle.Field(ai.CredFieldAWSSecretAccessKey),
		bundle.Field(ai.CredFieldAWSSessionToken),
	)
	return cfg
}
