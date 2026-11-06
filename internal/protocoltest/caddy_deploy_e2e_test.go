//go:build e2e

package protocoltest

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/peertransport"
)

// Exercise the checked-in deployment configuration, with only its host,
// backend address and certificates replaced by loopback test fixtures.
func TestCaddyDeploymentConfigHTTP3(t *testing.T) {
	bin := os.Getenv("CADDY_BIN")
	if bin == "" {
		t.Skip("set CADDY_BIN to run the Caddy deployment test")
	}
	certServer := httptest.NewTLSServer(http.NotFoundHandler())
	cert := certServer.TLS.Certificates[0]
	roots := x509.NewCertPool()
	roots.AddCert(certServer.Certificate())
	certServer.Close()
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
	canceled := make(chan struct{}, 1)
	backend := httptest.NewServer(peertransport.ProbeHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/tingly/test" && r.URL.Path != "/tingly/cancel" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: first\n\n")
		w.(http.Flusher).Flush()
		if r.URL.Path == "/tingly/cancel" {
			<-r.Context().Done()
			canceled <- struct{}{}
			return
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		w.(http.Flusher).Flush()
	})))
	t.Cleanup(backend.Close)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	raw, err := os.ReadFile("../../deploy/caddy/Caddyfile")
	if err != nil {
		t.Fatal(err)
	}
	config := strings.Replace(string(raw), "{\n", "{\n\tadmin off\n", 1)
	config = strings.Replace(config, "you.domain {", "https://"+addr+" {\n\ttls "+certFile+" "+keyFile, 1)
	config = strings.ReplaceAll(config, "localhost:12580", strings.TrimPrefix(backend.URL, "http://"))
	cfgPath := filepath.Join(dir, "Caddyfile")
	if err = os.WriteFile(cfgPath, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "run", "--config", cfgPath)
	cmd.Env = append(os.Environ(), "XDG_DATA_HOME="+dir, "XDG_CONFIG_HOME="+dir)
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	})
	deadline := time.Now().Add(5 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Caddy not ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = nil
	base.TLSClientConfig = &tls.Config{RootCAs: roots}
	peer := peertransport.New(base)
	defer peer.Close()
	client := &http.Client{Transport: peer, Timeout: 5 * time.Second}
	request := func(method, path string, auth bool) (*http.Response, error) {
		req, _ := http.NewRequest(method, "https://"+addr+path, nil)
		if auth {
			req.Header.Set("Authorization", "Bearer synthetic")
		}
		return client.Do(req)
	}
	var res *http.Response
	deadline = time.Now().Add(4 * time.Second)
	for {
		res, err = request(http.MethodPost, "/tingly/test", true)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil || res.StatusCode != 200 || !strings.Contains(string(body), "[DONE]") {
			t.Fatalf("stream status=%d body=%q err=%v", res.StatusCode, body, err)
		}
		if res.ProtoMajor == 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("deployment never selected HTTP/3")
		}
		time.Sleep(10 * time.Millisecond)
	}
	for path, want := range map[string]int{"/mirror/test": 200, "/hi": 200, "/blocked": 403} {
		res, err = request(http.MethodGet, path, true)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != want {
			t.Fatalf("%s status=%d want=%d", path, res.StatusCode, want)
		}
	}
	res, err = request(http.MethodPost, "/tingly/test", false)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("auth status=%d", res.StatusCode)
	}
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+addr+"/tingly/cancel", nil)
	req.Header.Set("Authorization", "Bearer synthetic")
	res, err = client.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	_, err = bufio.NewReader(res.Body).ReadString('\n')
	if err != nil {
		cancel()
		res.Body.Close()
		t.Fatal(err)
	}
	cancel()
	res.Body.Close()
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("Caddy did not propagate cancellation")
	}
	t.Log("checked-in Caddyfile: self-signed H3 discovery, SSE, cancellation, auth, mirror and default-deny routing passed")
}
