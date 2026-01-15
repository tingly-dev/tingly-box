package tls

import "time"

// TLSConfig holds TLS-related configuration
type TLSConfig struct {
	Enabled    bool   `json:"enabled" yaml:"enabled"`
	CertDir    string `json:"cert_dir" yaml:"cert_dir"`
	CertFile   string `json:"cert_file" yaml:"cert_file"`
	KeyFile    string `json:"key_file" yaml:"key_file"`
	Regenerate bool   `json:"regenerate" yaml:"regenerate"`
}

// CertificateInfo holds generated certificate metadata
type CertificateInfo struct {
	NotBefore time.Time `json:"not_before"`
	NotAfter  time.Time `json:"not_after"`
	Subject   string    `json:"subject"`
	DNSNames  []string  `json:"dns_names"`
}

// CertificateGenerator handles self-signed certificate generation
type CertificateGenerator struct {
	certDir  string
	certFile string
	keyFile  string
	infoFile string
}

// NewCertificateGenerator creates a new generator
func NewCertificateGenerator(certDir string) *CertificateGenerator {
	return &CertificateGenerator{
		certDir:  certDir,
		certFile: certDir + "/server.crt",
		keyFile:  certDir + "/server.key",
		infoFile: certDir + "/cert-info.json",
	}
}

// GetCertFile returns the certificate file path
func (g *CertificateGenerator) GetCertFile() string {
	return g.certFile
}

// GetKeyFile returns the private key file path
func (g *CertificateGenerator) GetKeyFile() string {
	return g.keyFile
}
