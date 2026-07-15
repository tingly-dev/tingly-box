package ai

import (
	"fmt"
	"strings"
)

// Canonical credential-bundle field keys for the multi-field auth types.
// These are the wire/storage keys inside CredentialBundle.Fields; both the
// outbound client (to build the SDK cloud adapter) and the HTTP handler (to
// validate and mask) reference them so the two stay in lock-step.
const (
	// AWS Bedrock (aws_sigv4)
	CredFieldAWSAccessKeyID     = "access_key_id"
	CredFieldAWSSecretAccessKey = "secret_access_key"
	CredFieldAWSSessionToken    = "session_token" // optional (STS/temporary creds)
	CredFieldAWSRegion          = "region"        // e.g. "us-east-1" — cloud region, not the template's geographic region
	CredFieldAWSBearerToken     = "bearer_token"  // optional alternative: Bedrock API key

	// GCP Vertex AI (gcp_sa)
	CredFieldGCPServiceAccountJSON = "service_account_json" // full SA key JSON (secret)
	CredFieldGCPProjectID          = "project_id"
	CredFieldGCPLocation           = "location" // e.g. "us-east5" / "global"

	// Azure OpenAI (azure_key)
	CredFieldAzureAPIKey     = "api_key"
	CredFieldAzureEndpoint   = "endpoint"    // e.g. "https://my-res.openai.azure.com"
	CredFieldAzureAPIVersion = "api_version" // e.g. "2024-10-21"
	CredFieldAzureDeployment = "deployment"  // optional; when model name != deployment
)

// CredentialFieldSpec describes one field in a multi-field credential schema.
type CredentialFieldSpec struct {
	Key      string `json:"key"`
	Required bool   `json:"required"`
	// Secret marks values that must be masked in API responses and never
	// logged (keys, secrets, session tokens, service-account JSON). Config
	// values (region, project, endpoint, api_version) are not secret.
	Secret bool `json:"secret"`
}

// CredentialSchema returns the ordered field specs for a multi-field auth
// type, or nil for single-field/bearer/oauth/vmodel types. The order is the
// suggested UI order. Requiredness here is the per-field baseline; auth types
// with conditional requirements (AWS: keys OR bearer) are enforced in
// ValidateCredential, not by these flags alone.
func CredentialSchema(a AuthType) []CredentialFieldSpec {
	switch a {
	case AuthTypeAWSSigV4:
		return []CredentialFieldSpec{
			{Key: CredFieldAWSRegion, Required: true},
			{Key: CredFieldAWSAccessKeyID},
			{Key: CredFieldAWSSecretAccessKey, Secret: true},
			{Key: CredFieldAWSSessionToken, Secret: true},
			{Key: CredFieldAWSBearerToken, Secret: true},
		}
	case AuthTypeGCPVertex:
		return []CredentialFieldSpec{
			{Key: CredFieldGCPProjectID, Required: true},
			{Key: CredFieldGCPLocation, Required: true},
			{Key: CredFieldGCPServiceAccountJSON, Required: true, Secret: true},
		}
	case AuthTypeAzureKey:
		return []CredentialFieldSpec{
			{Key: CredFieldAzureEndpoint, Required: true},
			{Key: CredFieldAzureAPIVersion, Required: true},
			{Key: CredFieldAzureAPIKey, Required: true, Secret: true},
			{Key: CredFieldAzureDeployment},
		}
	}
	return nil
}

// IsSecretCredentialField reports whether key holds a secret for authType and
// must therefore be masked in responses. Unknown keys are treated as secret
// (fail closed) so a new field is never accidentally leaked before its spec is
// added.
func IsSecretCredentialField(a AuthType, key string) bool {
	for _, f := range CredentialSchema(a) {
		if f.Key == key {
			return f.Secret
		}
	}
	return true
}

// ValidateCredential checks that fields satisfy the schema for a multi-field
// auth type. It returns nil for non-multi-field types (nothing to validate)
// and an error listing the first missing/invalid requirement otherwise.
func ValidateCredential(a AuthType, fields map[string]string) error {
	if !a.IsMultiFieldCredential() {
		return nil
	}
	get := func(k string) string { return strings.TrimSpace(fields[k]) }

	switch a {
	case AuthTypeAWSSigV4:
		if get(CredFieldAWSRegion) == "" {
			return fmt.Errorf("%s: %q is required", a, CredFieldAWSRegion)
		}
		hasKeys := get(CredFieldAWSAccessKeyID) != "" && get(CredFieldAWSSecretAccessKey) != ""
		hasBearer := get(CredFieldAWSBearerToken) != ""
		if !hasKeys && !hasBearer {
			return fmt.Errorf("%s: provide either %q+%q or %q",
				a, CredFieldAWSAccessKeyID, CredFieldAWSSecretAccessKey, CredFieldAWSBearerToken)
		}
		return nil
	default:
		for _, f := range CredentialSchema(a) {
			if f.Required && get(f.Key) == "" {
				return fmt.Errorf("%s: %q is required", a, f.Key)
			}
		}
		return nil
	}
}
