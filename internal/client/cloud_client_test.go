package client

import (
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func awsProvider(fields map[string]string) *typ.Provider {
	return &typ.Provider{
		Name:       "bedrock",
		AuthType:   typ.AuthTypeAWSSigV4,
		Credential: &typ.CredentialBundle{Fields: fields},
	}
}

func TestAWSConfigFromBundle_StaticKeys(t *testing.T) {
	cfg := awsConfigFromBundle(&typ.CredentialBundle{Fields: map[string]string{
		ai.CredFieldAWSRegion:          "eu-west-1",
		ai.CredFieldAWSAccessKeyID:     "AKIA",
		ai.CredFieldAWSSecretAccessKey: "secret",
	}})
	if cfg.Region != "eu-west-1" {
		t.Errorf("Region = %q, want eu-west-1", cfg.Region)
	}
	if cfg.Credentials == nil {
		t.Error("expected static Credentials provider to be set")
	}
	if cfg.BearerAuthTokenProvider != nil {
		t.Error("did not expect a bearer token provider for static keys")
	}
}

func TestAWSConfigFromBundle_BearerPreferred(t *testing.T) {
	cfg := awsConfigFromBundle(&typ.CredentialBundle{Fields: map[string]string{
		ai.CredFieldAWSRegion:      "us-east-1",
		ai.CredFieldAWSBearerToken: "bedrock-key",
		// keys present but bearer must win
		ai.CredFieldAWSAccessKeyID:     "AKIA",
		ai.CredFieldAWSSecretAccessKey: "secret",
	}})
	if cfg.BearerAuthTokenProvider == nil {
		t.Error("expected bearer token provider to be preferred")
	}
	if cfg.Credentials != nil {
		t.Error("did not expect static Credentials when bearer token is present")
	}
}

func TestBedrockOption_ValidationErrors(t *testing.T) {
	// missing bundle
	if _, err := bedrockOption(&typ.Provider{Name: "b", AuthType: typ.AuthTypeAWSSigV4}); err == nil {
		t.Error("expected error for missing credential bundle")
	}
	// missing region
	if _, err := bedrockOption(awsProvider(map[string]string{
		ai.CredFieldAWSAccessKeyID: "AKIA", ai.CredFieldAWSSecretAccessKey: "s",
	})); err == nil {
		t.Error("expected error for missing region")
	}
	// valid
	if _, err := bedrockOption(awsProvider(map[string]string{
		ai.CredFieldAWSRegion:          "us-east-1",
		ai.CredFieldAWSAccessKeyID:     "AKIA",
		ai.CredFieldAWSSecretAccessKey: "s",
	})); err != nil {
		t.Errorf("unexpected error for valid bundle: %v", err)
	}
}

func TestAzureOptions(t *testing.T) {
	// valid → two options (endpoint + api key)
	opts, err := azureOptions(&typ.Provider{
		Name:     "azure",
		AuthType: typ.AuthTypeAzureKey,
		Credential: &typ.CredentialBundle{Fields: map[string]string{
			ai.CredFieldAzureEndpoint:   "https://x.openai.azure.com",
			ai.CredFieldAzureAPIVersion: "2024-10-21",
			ai.CredFieldAzureAPIKey:     "key",
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(opts) != 2 {
		t.Errorf("len(opts) = %d, want 2", len(opts))
	}

	// missing api version → error
	if _, err := azureOptions(&typ.Provider{
		Name:     "azure",
		AuthType: typ.AuthTypeAzureKey,
		Credential: &typ.CredentialBundle{Fields: map[string]string{
			ai.CredFieldAzureEndpoint: "https://x", ai.CredFieldAzureAPIKey: "key",
		}},
	}); err == nil {
		t.Error("expected error for missing api version")
	}
}
