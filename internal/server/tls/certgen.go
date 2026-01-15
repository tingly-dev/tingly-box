package tls

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

const (
	// CertValidity is the certificate validity period
	CertValidity = 365 * 24 * time.Hour // 1 year
	// KeySize is the RSA key size
	KeySize = 2048
)

var (
	// DefaultDNSNames are the default DNS names for the certificate
	DefaultDNSNames = []string{"localhost", "127.0.0.1"}
)

// Exists checks if certificate files exist
func (g *CertificateGenerator) Exists() bool {
	if _, err := os.Stat(g.certFile); os.IsNotExist(err) {
		return false
	}
	if _, err := os.Stat(g.keyFile); os.IsNotExist(err) {
		return false
	}
	return true
}

// Load loads existing certificate info
func (g *CertificateGenerator) Load() (*CertificateInfo, error) {
	data, err := os.ReadFile(g.infoFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read cert info: %w", err)
	}

	var info CertificateInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("failed to parse cert info: %w", err)
	}

	return &info, nil
}

// ShouldRegenerate checks if certificate should be regenerated
func (g *CertificateGenerator) ShouldRegenerate() bool {
	info, err := g.Load()
	if err != nil {
		return true
	}

	// Regenerate if certificate is expired or will expire within 7 days
	return time.Until(info.NotAfter) < 7*24*time.Hour
}

// Generate generates a new self-signed certificate
func (g *CertificateGenerator) Generate() error {
	// Create certificate directory if it doesn't exist
	if err := os.MkdirAll(g.certDir, 0755); err != nil {
		return fmt.Errorf("failed to create cert directory: %w", err)
	}

	// Generate RSA private key
	privateKey, err := rsa.GenerateKey(rand.Reader, KeySize)
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template
	now := time.Now()
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Tingly-Box"},
			CommonName:   "localhost",
		},
		NotBefore:             now.UTC(),
		NotAfter:              now.Add(CertValidity).UTC(),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              DefaultDNSNames,
	}

	// Generate self-signed certificate
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return fmt.Errorf("failed to create certificate: %w", err)
	}

	// Write certificate to file
	certFile, err := os.Create(g.certFile)
	if err != nil {
		return fmt.Errorf("failed to create cert file: %w", err)
	}
	defer certFile.Close()

	if err := pem.Encode(certFile, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: derBytes,
	}); err != nil {
		return fmt.Errorf("failed to write certificate: %w", err)
	}

	// Write private key to file
	keyFile, err := os.OpenFile(g.keyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create key file: %w", err)
	}
	defer keyFile.Close()

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	if err := pem.Encode(keyFile, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	}); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}

	// Write certificate info
	info := CertificateInfo{
		NotBefore: template.NotBefore,
		NotAfter:  template.NotAfter,
		Subject:   template.Subject.CommonName,
		DNSNames:  template.DNSNames,
	}

	infoData, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cert info: %w", err)
	}

	if err := os.WriteFile(g.infoFile, infoData, 0644); err != nil {
		return fmt.Errorf("failed to write cert info: %w", err)
	}

	return nil
}

// EnsureCertificates ensures certificates exist and are valid
func (g *CertificateGenerator) EnsureCertificates(regenerate bool) error {
	if g.Exists() && !regenerate {
		// Check if certificate needs regeneration
		if !g.ShouldRegenerate() {
			return nil
		}
	}

	return g.Generate()
}

// GetDefaultCertDir returns the default certificate directory
func GetDefaultCertDir(configDir string) string {
	return filepath.Join(configDir, "certs")
}
