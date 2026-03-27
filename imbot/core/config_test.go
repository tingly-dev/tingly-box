package core

import (
	"os"
	"testing"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid token config",
			config: &Config{
				Platform: PlatformTelegram,
				Enabled:  true,
				Auth: AuthConfig{
					Type:  "token",
					Token: "test-token",
				},
			},
			wantErr: false,
		},
		{
			name: "Invalid platform",
			config: &Config{
				Platform: Platform("invalid"),
				Auth: AuthConfig{
					Type:  "token",
					Token: "test-token",
				},
			},
			wantErr: true,
			errMsg:  "invalid platform",
		},
		{
			name: "Token auth without token",
			config: &Config{
				Platform: PlatformTelegram,
				Auth: AuthConfig{
					Type: "token",
				},
			},
			wantErr: true,
			errMsg:  "token is required",
		},
		{
			name: "Basic auth without username",
			config: &Config{
				Platform: PlatformTelegram,
				Auth: AuthConfig{
					Type: "basic",
				},
			},
			wantErr: true,
			errMsg:  "username is required",
		},
		{
			name: "OAuth without credentials",
			config: &Config{
				Platform: PlatformTelegram,
				Auth: AuthConfig{
					Type: "oauth",
				},
			},
			wantErr: true,
			errMsg:  "clientId and clientSecret are required",
		},
		{
			name: "Service account without JSON",
			config: &Config{
				Platform: PlatformGoogleChat,
				Auth: AuthConfig{
					Type: "serviceAccount",
				},
			},
			wantErr: true,
			errMsg:  "serviceAccountJson is required",
		},
		{
			name: "Unknown auth type",
			config: &Config{
				Platform: PlatformTelegram,
				Auth: AuthConfig{
					Type: "unknown",
				},
			},
			wantErr: true,
			errMsg:  "unknown auth type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" {
				if !containsSubstring(err.Error(), tt.errMsg) {
					t.Errorf("Error message should contain %v, got %v", tt.errMsg, err.Error())
				}
			}
		})
	}
}

func TestAuthConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		auth    AuthConfig
		wantErr bool
	}{
		{
			name: "Valid token auth",
			auth: AuthConfig{
				Type:  "token",
				Token: "test-token",
			},
			wantErr: false,
		},
		{
			name: "Token auth without token",
			auth: AuthConfig{
				Type: "token",
			},
			wantErr: true,
		},
		{
			name: "Valid basic auth",
			auth: AuthConfig{
				Type:     "basic",
				Username: "user",
				Password: "pass",
			},
			wantErr: false,
		},
		{
			name: "Basic auth without username",
			auth: AuthConfig{
				Type: "basic",
			},
			wantErr: true,
		},
		{
			name: "Valid oauth",
			auth: AuthConfig{
				Type:         "oauth",
				ClientID:     "client-id",
				ClientSecret: "client-secret",
			},
			wantErr: false,
		},
		{
			name: "OAuth without client ID",
			auth: AuthConfig{
				Type:         "oauth",
				ClientSecret: "secret",
			},
			wantErr: true,
		},
		{
			name: "Valid service account",
			auth: AuthConfig{
				Type:               "serviceAccount",
				ServiceAccountJSON: "{}",
			},
			wantErr: false,
		},
		{
			name: "Service account without JSON",
			auth: AuthConfig{
				Type: "serviceAccount",
			},
			wantErr: true,
		},
		{
			name: "QR auth (no required fields)",
			auth: AuthConfig{
				Type: "qr",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.auth.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("AuthConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_ExpandEnvVars(t *testing.T) {
	// Set test environment variables
	os.Setenv("TEST_TOKEN", "env-token-value")
	os.Setenv("TEST_PASSWORD", "env-password-value")
	os.Setenv("TEST_CLIENT_ID", "env-client-id")
	os.Setenv("TEST_CLIENT_SECRET", "env-client-secret")
	os.Setenv("TEST_SERVICE_ACCOUNT", `{"key":"value"}`)
	defer func() {
		os.Unsetenv("TEST_TOKEN")
		os.Unsetenv("TEST_PASSWORD")
		os.Unsetenv("TEST_CLIENT_ID")
		os.Unsetenv("TEST_CLIENT_SECRET")
		os.Unsetenv("TEST_SERVICE_ACCOUNT")
	}()

	tests := []struct {
		name           string
		config         *Config
		expectedToken  string
		expectedPass   string
		expectedClient string
		expectedSecret string
		expectedSA     string
	}{
		{
			name: "Expand token",
			config: &Config{
				Platform: PlatformTelegram,
				Auth: AuthConfig{
					Type:  "token",
					Token: "$TEST_TOKEN",
				},
			},
			expectedToken: "env-token-value",
		},
		{
			name: "Expand password",
			config: &Config{
				Platform: PlatformTelegram,
				Auth: AuthConfig{
					Type:     "basic",
					Username: "user",
					Password: "$TEST_PASSWORD",
				},
			},
			expectedPass: "env-password-value",
		},
		{
			name: "Expand OAuth credentials",
			config: &Config{
				Platform: PlatformGoogleChat,
				Auth: AuthConfig{
					Type:         "oauth",
					ClientID:     "$TEST_CLIENT_ID",
					ClientSecret: "$TEST_CLIENT_SECRET",
				},
			},
			expectedClient: "env-client-id",
			expectedSecret: "env-client-secret",
		},
		{
			name: "Expand service account",
			config: &Config{
				Platform: PlatformGoogleChat,
				Auth: AuthConfig{
					Type:               "serviceAccount",
					ServiceAccountJSON: "$TEST_SERVICE_ACCOUNT",
				},
			},
			expectedSA: `{"key":"value"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.config.ExpandEnvVars()

			if tt.expectedToken != "" && tt.config.Auth.Token != tt.expectedToken {
				t.Errorf("Token = %v, want %v", tt.config.Auth.Token, tt.expectedToken)
			}

			if tt.expectedPass != "" && tt.config.Auth.Password != tt.expectedPass {
				t.Errorf("Password = %v, want %v", tt.config.Auth.Password, tt.expectedPass)
			}

			if tt.expectedClient != "" && tt.config.Auth.ClientID != tt.expectedClient {
				t.Errorf("ClientID = %v, want %v", tt.config.Auth.ClientID, tt.expectedClient)
			}

			if tt.expectedSecret != "" && tt.config.Auth.ClientSecret != tt.expectedSecret {
				t.Errorf("ClientSecret = %v, want %v", tt.config.Auth.ClientSecret, tt.expectedSecret)
			}

			if tt.expectedSA != "" && tt.config.Auth.ServiceAccountJSON != tt.expectedSA {
				t.Errorf("ServiceAccountJSON = %v, want %v", tt.config.Auth.ServiceAccountJSON, tt.expectedSA)
			}
		})
	}
}

func TestConfig_GetOptionString(t *testing.T) {
	config := &Config{
		Platform: PlatformTelegram,
		Options: map[string]interface{}{
			"stringOption": "value",
			"intOption":    42,
		},
	}

	tests := []struct {
		name         string
		key          string
		defaultValue string
		want         string
	}{
		{
			name:         "Existing string option",
			key:          "stringOption",
			defaultValue: "default",
			want:         "value",
		},
		{
			name:         "Non-existing option",
			key:          "nonExisting",
			defaultValue: "default",
			want:         "default",
		},
		{
			name:         "Option with wrong type",
			key:          "intOption",
			defaultValue: "default",
			want:         "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := config.GetOptionString(tt.key, tt.defaultValue); got != tt.want {
				t.Errorf("GetOptionString(%v, %v) = %v, want %v", tt.key, tt.defaultValue, got, tt.want)
			}
		})
	}
}

func TestConfig_GetOptionBool(t *testing.T) {
	config := &Config{
		Platform: PlatformTelegram,
		Options: map[string]interface{}{
			"boolOption":   true,
			"stringOption": "true",
		},
	}

	tests := []struct {
		name         string
		key          string
		defaultValue bool
		want         bool
	}{
		{
			name:         "Existing bool option",
			key:          "boolOption",
			defaultValue: false,
			want:         true,
		},
		{
			name:         "Non-existing option",
			key:          "nonExisting",
			defaultValue: true,
			want:         true,
		},
		{
			name:         "Option with wrong type",
			key:          "stringOption",
			defaultValue: false,
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := config.GetOptionBool(tt.key, tt.defaultValue); got != tt.want {
				t.Errorf("GetOptionBool(%v, %v) = %v, want %v", tt.key, tt.defaultValue, got, tt.want)
			}
		})
	}
}

func TestConfig_GetOptionInt(t *testing.T) {
	config := &Config{
		Platform: PlatformTelegram,
		Options: map[string]interface{}{
			"intOption":    42,
			"floatOption":  42.5,
			"stringOption": "42",
		},
	}

	tests := []struct {
		name         string
		key          string
		defaultValue int
		want         int
	}{
		{
			name:         "Existing int option",
			key:          "intOption",
			defaultValue: 0,
			want:         42,
		},
		{
			name:         "Float option",
			key:          "floatOption",
			defaultValue: 0,
			want:         42,
		},
		{
			name:         "Non-existing option",
			key:          "nonExisting",
			defaultValue: 10,
			want:         10,
		},
		{
			name:         "Option with wrong type",
			key:          "stringOption",
			defaultValue: 10,
			want:         10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := config.GetOptionInt(tt.key, tt.defaultValue); got != tt.want {
				t.Errorf("GetOptionInt(%v, %v) = %v, want %v", tt.key, tt.defaultValue, got, tt.want)
			}
		})
	}
}

func TestConfig_Clone(t *testing.T) {
	original := &Config{
		Platform: PlatformTelegram,
		Enabled:  true,
		Auth: AuthConfig{
			Type:  "token",
			Token: "test-token",
		},
		Options: map[string]interface{}{
			"key1": "value1",
			"key2": 42,
		},
		Logging: &LoggingConfig{
			Level:      "debug",
			Timestamps: true,
		},
	}

	clone := original.Clone()

	// Check that values match
	if clone.Platform != original.Platform {
		t.Errorf("Clone Platform = %v, want %v", clone.Platform, original.Platform)
	}

	if clone.Auth.Token != original.Auth.Token {
		t.Errorf("Clone Token = %v, want %v", clone.Auth.Token, original.Auth.Token)
	}

	// Modify original options and check clone is unaffected
	original.Options["key1"] = "modified"
	if clone.Options["key1"] == "modified" {
		t.Error("Clone options was affected by modification to original")
	}

	// Modify original logging and check clone is not affected (if it was cloned deeply)
	original.Logging.Level = "error"
	if clone.Logging.Level == "error" && original.Logging != clone.Logging {
		// This is actually correct behavior - logging should be cloned
	} else if clone.Logging.Level == "error" {
		t.Error("Clone logging was affected by modification to original")
	}
}

func TestConfigs_Validate(t *testing.T) {
	tests := []struct {
		name    string
		configs *Configs
		wantErr bool
	}{
		{
			name: "All valid configs",
			configs: &Configs{
				Bots: []*Config{
					{
						Platform: PlatformTelegram,
						Auth: AuthConfig{
							Type:  "token",
							Token: "token1",
						},
					},
					{
						Platform: PlatformDiscord,
						Auth: AuthConfig{
							Type:  "token",
							Token: "token2",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "One invalid config",
			configs: &Configs{
				Bots: []*Config{
					{
						Platform: PlatformTelegram,
						Auth: AuthConfig{
							Type:  "token",
							Token: "token1",
						},
					},
					{
						Platform: PlatformTelegram,
						Auth: AuthConfig{
							Type: "token", // Missing token
						},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.configs.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Configs.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfigs_GetEnabledConfigs(t *testing.T) {
	configs := &Configs{
		Bots: []*Config{
			{
				Platform: PlatformTelegram,
				Enabled:  true,
				Auth: AuthConfig{
					Type:  "token",
					Token: "token1",
				},
			},
			{
				Platform: PlatformDiscord,
				Enabled:  false,
				Auth: AuthConfig{
					Type:  "token",
					Token: "token2",
				},
			},
			{
				Platform: PlatformSlack,
				Enabled:  true,
				Auth: AuthConfig{
					Type:  "token",
					Token: "token3",
				},
			},
		},
	}

	enabled := configs.GetEnabledConfigs()

	if len(enabled) != 2 {
		t.Errorf("GetEnabledConfigs() returned %d configs, want 2", len(enabled))
	}

	for _, cfg := range enabled {
		if !cfg.Enabled {
			t.Errorf("Config should be enabled, got enabled=%v", cfg.Enabled)
		}
	}
}

func TestDefaultManagerConfig(t *testing.T) {
	config := DefaultManagerConfig()

	if !config.AutoReconnect {
		t.Error("Default AutoReconnect should be true")
	}

	if config.MaxReconnectAttempts != 5 {
		t.Errorf("Default MaxReconnectAttempts = %v, want 5", config.MaxReconnectAttempts)
	}

	if config.ReconnectDelayMs != 5000 {
		t.Errorf("Default ReconnectDelayMs = %v, want 5000", config.ReconnectDelayMs)
	}
}
