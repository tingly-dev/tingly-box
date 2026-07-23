package protocoltest

import (
	"strings"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/server"
)

// The duoInstanceSpec is a cross-process contract (parent env → child boot),
// so its round-trip and validation behaviour are pinned here without booting
// any child process.

func TestDuoSpecRoundTrip(t *testing.T) {
	spec := duoInstanceSpec{
		Name:          "tb2",
		Role:          duoRoleGateway,
		ConfigDir:     "/tmp/duo-tb2",
		Port:          4242,
		UpstreamURL:   "http://127.0.0.1:4141",
		UpstreamToken: "sk-tingly-upstream",
		HTTPTimeouts:  server.HTTPTimeouts{WriteTimeout: 300 * time.Millisecond},
		Stream:        DuoStreamShape{SizeKB: 64, Delay: 150 * time.Millisecond},
	}
	entry, err := spec.encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	raw, ok := strings.CutPrefix(entry, duoEnvSpec+"=")
	if !ok {
		t.Fatalf("env entry %q not prefixed with %s=", entry, duoEnvSpec)
	}
	got, err := decodeDuoSpec(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got != spec {
		t.Errorf("round trip mismatch:\n got  %+v\n want %+v", got, spec)
	}
}

func TestDuoSpecValidate(t *testing.T) {
	upstream := duoInstanceSpec{Name: "tb1", Role: duoRoleUpstream, ConfigDir: "/tmp/duo-tb1", Port: 4141}
	gateway := duoInstanceSpec{
		Name: "tb2", Role: duoRoleGateway, ConfigDir: "/tmp/duo-tb2", Port: 4242,
		UpstreamURL: "http://127.0.0.1:4141", UpstreamToken: "sk-tingly-upstream",
	}

	cases := []struct {
		name    string
		mutate  func(*duoInstanceSpec)
		spec    duoInstanceSpec
		wantErr string // "" = valid
	}{
		{name: "upstream ok", spec: upstream},
		{name: "gateway ok", spec: gateway},
		{name: "unknown role", spec: upstream, mutate: func(s *duoInstanceSpec) { s.Role = "sidecar" }, wantErr: "unknown duo role"},
		{name: "missing config dir", spec: upstream, mutate: func(s *duoInstanceSpec) { s.ConfigDir = "" }, wantErr: "config_dir"},
		{name: "missing port", spec: gateway, mutate: func(s *duoInstanceSpec) { s.Port = 0 }, wantErr: "port"},
		{name: "gateway without upstream", spec: gateway, mutate: func(s *duoInstanceSpec) { s.UpstreamURL = ""; s.UpstreamToken = "" }, wantErr: "upstream_url, upstream_token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := tc.spec
			if tc.mutate != nil {
				tc.mutate(&spec)
			}
			err := spec.validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validate: unexpected error %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("validate: got %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}
