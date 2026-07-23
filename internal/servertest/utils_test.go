package servertest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestParseRealConfigProviderTimeout(t *testing.T) {
	testCases := []struct {
		name    string
		value   string
		want    int64
		wantErr bool
	}{
		{name: "default", want: defaultRealConfigProviderTimeoutSeconds},
		{name: "configured", value: "3", want: 3},
		{name: "zero", value: "0", wantErr: true},
		{name: "negative", value: "-1", wantErr: true},
		{name: "invalid", value: "slow", wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := parseRealConfigProviderTimeout(testCase.value)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != testCase.want {
				t.Fatalf("got %d seconds, want %d", got, testCase.want)
			}
		})
	}
}

func TestSnapshotRealConfigCopiesOnlyConfigurationAndProviders(t *testing.T) {
	sourceDir := t.TempDir()
	destinationDir := t.TempDir()

	configData := []byte(`{"rules":[]}`)
	if err := os.WriteFile(filepath.Join(sourceDir, "config.json"), configData, 0600); err != nil {
		t.Fatalf("write source config: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(sourceDir, "record"), 0700); err != nil {
		t.Fatalf("create record directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "record", "large-runtime-artifact"), []byte("not config"), 0600); err != nil {
		t.Fatalf("write runtime artifact: %v", err)
	}

	sourceProviders, err := db.NewProviderStore(sourceDir)
	if err != nil {
		t.Fatalf("create source provider store: %v", err)
	}
	if err := sourceProviders.Save(&typ.Provider{
		UUID:    "provider-1",
		Name:    "provider-1",
		APIBase: "https://example.com",
		Token:   "test-token",
		Timeout: 1800,
		Enabled: true,
	}); err != nil {
		t.Fatalf("save source provider: %v", err)
	}
	if err := sourceProviders.Close(); err != nil {
		t.Fatalf("close source provider store: %v", err)
	}

	if err := snapshotRealConfig(sourceDir, destinationDir, 4); err != nil {
		t.Fatalf("snapshot real config: %v", err)
	}

	copiedConfig, err := os.ReadFile(filepath.Join(destinationDir, "config.json"))
	if err != nil {
		t.Fatalf("read copied config: %v", err)
	}
	if string(copiedConfig) != string(configData) {
		t.Fatalf("copied config mismatch: got %q, want %q", copiedConfig, configData)
	}
	if _, err := os.Stat(filepath.Join(destinationDir, "record")); !os.IsNotExist(err) {
		t.Fatalf("runtime record directory should not be copied, stat error: %v", err)
	}

	destinationProviders, err := db.NewProviderStore(destinationDir)
	if err != nil {
		t.Fatalf("open destination provider store: %v", err)
	}
	defer destinationProviders.Close()

	providers, err := destinationProviders.List()
	if err != nil {
		t.Fatalf("list destination providers: %v", err)
	}
	if len(providers) != 1 {
		t.Fatalf("got %d providers, want 1", len(providers))
	}
	if providers[0].Timeout != 4 {
		t.Fatalf("provider timeout: got %d, want 4", providers[0].Timeout)
	}
}
