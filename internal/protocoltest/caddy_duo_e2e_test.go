//go:build e2e

package protocoltest

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// CADDY_BIN selects a local Caddy binary for the two-process TB harness.
func TestCaddySelfSignedTwoRealTB(t *testing.T) {
	bin := os.Getenv("CADDY_BIN")
	if bin == "" {
		t.Skip("set CADDY_BIN to run the Caddy end-to-end test")
	}
	fixture := httptest.NewTLSServer(http.NotFoundHandler())
	cert := fixture.TLS.Certificates[0]
	fixture.Close()
	key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err = os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSL_CERT_FILE", certFile)
	cfg := DuoEnvConfig{BootTimeout: 60 * time.Second}
	cfg.withDefaults()
	env := &DuoEnv{client: &http.Client{Timeout: 20 * time.Second}}
	defer env.Close()
	tb1, err := env.startInstance(duoInstanceSpec{Name: "cloud", Role: duoRoleUpstream, Stream: cfg.Upstream.Stream}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	env.TB1 = tb1
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	backend := strings.TrimPrefix(tb1.BaseURL, "http://")
	makeCaddy := func(protocols []string) func() {
		config := map[string]any{"admin": map[string]any{"disabled": true}, "logging": map[string]any{"logs": map[string]any{"default": map[string]any{"level": "ERROR"}}}, "apps": map[string]any{
			"tls":  map[string]any{"certificates": map[string]any{"load_files": []any{map[string]any{"certificate": certFile, "key": keyFile}}}},
			"http": map[string]any{"servers": map[string]any{"test": map[string]any{"listen": []string{addr}, "protocols": protocols, "tls_connection_policies": []any{map[string]any{}}, "automatic_https": map[string]any{"disable": true}, "routes": []any{map[string]any{"handle": []any{map[string]any{"handler": "reverse_proxy", "upstreams": []any{map[string]any{"dial": backend}}}}}}}}},
		}}
		raw, _ := json.Marshal(config)
		file := filepath.Join(t.TempDir(), "caddy.json")
		if err := os.WriteFile(file, raw, 0600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(bin, "run", "--config", file)
		cmd.Env = append(os.Environ(), "XDG_DATA_HOME="+dir, "XDG_CONFIG_HOME="+dir)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		deadline := time.Now().Add(5 * time.Second)
		for {
			conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
			if err == nil {
				conn.Close()
				break
			}
			select {
			case err := <-done:
				t.Fatalf("Caddy exited: %v", err)
			default:
			}
			if time.Now().After(deadline) {
				cmd.Process.Kill()
				t.Fatal("Caddy not ready")
			}
			time.Sleep(10 * time.Millisecond)
		}
		stopped := false
		return func() {
			if stopped {
				return
			}
			stopped = true
			_ = cmd.Process.Signal(os.Interrupt)
			select {
			case <-done:
			case <-time.After(4 * time.Second):
				_ = cmd.Process.Kill()
				<-done
			}
		}
	}
	stop := makeCaddy([]string{"h1", "h2", "h3"})
	defer func() { stop() }()
	// The same private self-signed CA must be trusted by the gateway process.
	tb2, err := env.startInstance(duoInstanceSpec{Name: "local", Role: duoRoleGateway, UpstreamURL: "https://" + addr, UpstreamToken: tb1.ModelToken}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	env.TB2 = tb2
	run := func(route DuoRoute) bool {
		checks := env.RunFunctionalChecks(route, 4096)
		for _, check := range checks {
			if !check.Pass {
				t.Logf("%s %s", check.Name, check.Detail)
				return false
			}
		}
		return len(checks) > 0
	}
	seen := func(provider, transport string) bool {
		for _, line := range strings.Split(tb2.OutputTail(), "\n") {
			if strings.Contains(line, "provider="+provider) && strings.Contains(line, "http_protocol="+transport) {
				return true
			}
		}
		return false
	}
	for _, route := range []DuoRoute{{Name: "v1-chat", Target: "chat"}, {Name: "v1-anthropic", Target: "anthropic"}} {
		deadline := time.Now().Add(6 * time.Second)
		for {
			if !run(route) {
				t.Fatal("two-TB streaming/non-streaming checks failed")
			}
			if seen(map[string]string{"chat": "tb1-openai-chat", "anthropic": "tb1-anthropic"}[route.Target], "HTTP/3.0") {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("gateway never selected HTTP/3")
			}
			time.Sleep(30 * time.Millisecond)
		}
		t.Logf("%s: streaming, non-streaming, usage and H3 checks passed", route.Name)
	}
	if !strings.Contains(tb2.OutputTail(), "http_protocol=HTTP/2.0") {
		t.Fatal("initial HTTPS/TCP path not observed")
	}
	t.Log("gateway observed initial HTTP/2 followed by HTTP/3 through Caddy")
	beforeFallback := strings.Count(tb2.OutputTail(), "http_protocol=HTTP/2.0")
	stop()
	stop = makeCaddy([]string{"h1", "h2"})
	route := DuoRoute{Name: "v1-chat", Target: "chat"}
	// The first post-interruption request may fail; never demand implicit replay.
	// A later request must recover on the existing HTTPS/TCP path.
	deadline := time.Now().Add(12 * time.Second)
	recovered := false
	for time.Now().Before(deadline) {
		if run(route) {
			recovered = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !recovered || strings.Count(tb2.OutputTail(), "http_protocol=HTTP/2.0") <= beforeFallback {
		t.Fatal("TCP fallback did not recover on HTTP/2")
	}
	t.Log("after Caddy H3 was disabled, a subsequent request recovered via TCP")
}
