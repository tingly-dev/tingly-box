package ai

import "testing"

func TestValidateCredential(t *testing.T) {
	tests := []struct {
		name    string
		auth    AuthType
		fields  map[string]string
		wantErr bool
	}{
		{
			name:   "non-multi-field is a no-op",
			auth:   AuthTypeAPIKey,
			fields: nil,
		},
		{
			name: "aws static keys ok",
			auth: AuthTypeAWSSigV4,
			fields: map[string]string{
				CredFieldAWSRegion:          "us-east-1",
				CredFieldAWSAccessKeyID:     "AKIA",
				CredFieldAWSSecretAccessKey: "secret",
			},
		},
		{
			name: "aws bearer token ok without keys",
			auth: AuthTypeAWSSigV4,
			fields: map[string]string{
				CredFieldAWSRegion:      "us-east-1",
				CredFieldAWSBearerToken: "bedrock-key",
			},
		},
		{
			name:    "aws missing region",
			auth:    AuthTypeAWSSigV4,
			fields:  map[string]string{CredFieldAWSAccessKeyID: "AKIA", CredFieldAWSSecretAccessKey: "s"},
			wantErr: true,
		},
		{
			name:    "aws missing both keys and bearer",
			auth:    AuthTypeAWSSigV4,
			fields:  map[string]string{CredFieldAWSRegion: "us-east-1"},
			wantErr: true,
		},
		{
			name:    "aws secret-only is incomplete",
			auth:    AuthTypeAWSSigV4,
			fields:  map[string]string{CredFieldAWSRegion: "us-east-1", CredFieldAWSAccessKeyID: "AKIA"},
			wantErr: true,
		},
		{
			name: "gcp complete",
			auth: AuthTypeGCPVertex,
			fields: map[string]string{
				CredFieldGCPProjectID:          "proj",
				CredFieldGCPLocation:           "us-east5",
				CredFieldGCPServiceAccountJSON: `{"type":"service_account"}`,
			},
		},
		{
			name:    "gcp missing project",
			auth:    AuthTypeGCPVertex,
			fields:  map[string]string{CredFieldGCPLocation: "us-east5", CredFieldGCPServiceAccountJSON: "{}"},
			wantErr: true,
		},
		{
			name: "azure complete",
			auth: AuthTypeAzureKey,
			fields: map[string]string{
				CredFieldAzureEndpoint:   "https://x.openai.azure.com",
				CredFieldAzureAPIVersion: "2024-10-21",
				CredFieldAzureAPIKey:     "key",
			},
		},
		{
			name:    "azure missing api version",
			auth:    AuthTypeAzureKey,
			fields:  map[string]string{CredFieldAzureEndpoint: "https://x", CredFieldAzureAPIKey: "key"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCredential(tt.auth, tt.fields)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateCredential() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsSecretCredentialField(t *testing.T) {
	cases := []struct {
		auth AuthType
		key  string
		want bool
	}{
		{AuthTypeAWSSigV4, CredFieldAWSRegion, false},
		{AuthTypeAWSSigV4, CredFieldAWSAccessKeyID, false},
		{AuthTypeAWSSigV4, CredFieldAWSSecretAccessKey, true},
		{AuthTypeAWSSigV4, CredFieldAWSSessionToken, true},
		{AuthTypeAWSSigV4, CredFieldAWSBearerToken, true},
		{AuthTypeGCPVertex, CredFieldGCPProjectID, false},
		{AuthTypeGCPVertex, CredFieldGCPServiceAccountJSON, true},
		{AuthTypeAzureKey, CredFieldAzureEndpoint, false},
		{AuthTypeAzureKey, CredFieldAzureAPIKey, true},
		// Unknown key fails closed (treated as secret).
		{AuthTypeAWSSigV4, "unknown_field", true},
	}
	for _, c := range cases {
		if got := IsSecretCredentialField(c.auth, c.key); got != c.want {
			t.Errorf("IsSecretCredentialField(%s, %q) = %v, want %v", c.auth, c.key, got, c.want)
		}
	}
}
