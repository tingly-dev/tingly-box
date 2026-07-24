package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type EnterpriseContextPublicKey struct {
	KID string `json:"kid" yaml:"kid"`
	PEM string `json:"pem" yaml:"pem"`
}

type EnterpriseContextJWTConfig struct {
	Enabled           bool                         `json:"enabled" yaml:"enabled"`
	AllowedIssuers    []string                     `json:"allowed_issuers,omitempty" yaml:"allowed_issuers,omitempty"`
	AllowedAudiences  []string                     `json:"allowed_audiences,omitempty" yaml:"allowed_audiences,omitempty"`
	AlgAllowlist      []string                     `json:"alg_allowlist,omitempty" yaml:"alg_allowlist,omitempty"`
	HS256SecretRef    string                       `json:"hs256_secret_ref,omitempty" yaml:"hs256_secret_ref,omitempty"`
	RS256PublicKeyRef string                       `json:"rs256_public_key_ref,omitempty" yaml:"rs256_public_key_ref,omitempty"`
	JWKSURL           string                       `json:"jwks_url,omitempty" yaml:"jwks_url,omitempty"`
	PublicKeys        []EnterpriseContextPublicKey `json:"public_keys,omitempty" yaml:"public_keys,omitempty"`
	ClockSkewSeconds  int                          `json:"clock_skew_seconds,omitempty" yaml:"clock_skew_seconds,omitempty"`
	RequireJTI        bool                         `json:"require_jti" yaml:"require_jti"`
}

func enterpriseContextKeyPaths(configDir string) (string, string) {
	keyDir := filepath.Join(configDir, "keys")
	return filepath.Join(keyDir, "enterprise_context_rs256_private.pem"),
		filepath.Join(keyDir, "enterprise_context_rs256_public.pem")
}

func ensureEnterpriseContextRS256KeyPair(configDir string) (string, string, error) {
	privatePath, publicPath := enterpriseContextKeyPaths(configDir)
	privateOK := false
	publicOK := false
	if st, err := os.Stat(privatePath); err == nil && !st.IsDir() {
		privateOK = true
	}
	if st, err := os.Stat(publicPath); err == nil && !st.IsDir() {
		publicOK = true
	}
	if privateOK && publicOK {
		return "file:" + privatePath, "file:" + publicPath, nil
	}

	privatePEM, publicPEM, err := generateEnterpriseContextPEMs()
	if err != nil {
		return "", "", err
	}
	if err := writeEnterpriseContextKeys(privatePath, publicPath, privatePEM, publicPEM); err != nil {
		return "", "", err
	}
	return "file:" + privatePath, "file:" + publicPath, nil
}

// generateEnterpriseContextPEMs generates a fresh RSA-2048 key pair and
// returns it PEM-encoded (private PKCS1, public PKIX).
func generateEnterpriseContextPEMs() (privatePEM, publicPEM []byte, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("generate rsa key failed: %w", err)
	}
	privatePEM = pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	pubDer, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal rsa public key failed: %w", err)
	}
	publicPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubDer,
	})
	return privatePEM, publicPEM, nil
}

// writeEnterpriseContextKeys writes a PEM key pair into configDir's key slots.
func writeEnterpriseContextKeys(privatePath, publicPath string, privatePEM, publicPEM []byte) error {
	if err := os.MkdirAll(filepath.Dir(privatePath), 0700); err != nil {
		return fmt.Errorf("create enterprise key dir failed: %w", err)
	}
	if err := os.WriteFile(privatePath, privatePEM, 0600); err != nil {
		return fmt.Errorf("write enterprise private key failed: %w", err)
	}
	if err := os.WriteFile(publicPath, publicPEM, 0644); err != nil {
		return fmt.Errorf("write enterprise public key failed: %w", err)
	}
	return nil
}

var (
	preseedKeyOnce sync.Once
	preseedPrivate []byte
	preseedPublic  []byte
	preseedErr     error
)

// PreseedEnterpriseContextKeys writes a process-cached RSA-2048 key pair into
// configDir's enterprise-context key slots, so that config creation skips the
// expensive per-directory key generation (~100-600ms of prime search).
//
// Intended for test harnesses that boot many fresh config dirs in one process
// — the generated key never leaves the process's temp dirs and every harness
// env sharing one throwaway key is deliberate. A no-op if the slots already
// hold keys. Production configs never call this: they generate once and reuse
// the key from disk.
func PreseedEnterpriseContextKeys(configDir string) error {
	privatePath, publicPath := enterpriseContextKeyPaths(configDir)
	if _, err := os.Stat(privatePath); err == nil {
		return nil
	}
	preseedKeyOnce.Do(func() {
		preseedPrivate, preseedPublic, preseedErr = generateEnterpriseContextPEMs()
	})
	if preseedErr != nil {
		return preseedErr
	}
	return writeEnterpriseContextKeys(privatePath, publicPath, preseedPrivate, preseedPublic)
}
