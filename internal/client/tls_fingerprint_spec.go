package client

import utls "github.com/refraction-networking/utls"

// GetAntigravityHelloSpec returns the ClientHelloSpec for Antigravity Windows client
// Antigravity connects to cloudcode-pa.googleapis.com
// Target: Standard TLS 1.3 client fingerprint (similar to curl/openssl)
//
// Verified connection details:
//   - Protocol: TLSv1.3
//   - Cipher: TLS_AES_256_GCM_SHA384 (server selected)
//   - Key Exchange: X25519
//   - ALPN: h2
//   - Server: cloudcode-pa.googleapis.com
func GetAntigravityHelloSpec() *utls.ClientHelloSpec {
	return &utls.ClientHelloSpec{
		// TLS 1.3 + TLS 1.2 cipher suites (in order of preference)
		CipherSuites: []uint16{
			// TLS 1.3
			utls.TLS_AES_128_GCM_SHA256,
			utls.TLS_AES_256_GCM_SHA384,
			utls.TLS_CHACHA20_POLY1305_SHA256,
			// TLS 1.2 ECDHE
			utls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			utls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			utls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			utls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			utls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			utls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
			// TLS 1.2 RSA
			utls.TLS_RSA_WITH_AES_128_GCM_SHA256,
			utls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		},
		CompressionMethods: []byte{0x00}, // null compression
		Extensions: []utls.TLSExtension{
			// Server Name Indication
			&utls.SNIExtension{},
			// Application-Layer Protocol Negotiation (HTTP/2, HTTP/1.1)
			&utls.ALPNExtension{
				AlpnProtocols: []string{"h2", "http/1.1"},
			},
			// Supported TLS versions
			&utls.SupportedVersionsExtension{
				Versions: []uint16{
					utls.VersionTLS13,
					utls.VersionTLS12,
				},
			},
			// Supported elliptic curves (key exchange groups)
			&utls.SupportedCurvesExtension{
				Curves: []utls.CurveID{
					utls.X25519,
					utls.CurveP256,
					utls.CurveP384,
				},
			},
			// Supported point formats
			&utls.SupportedPointsExtension{
				SupportedPoints: []byte{0x00}, // uncompressed
			},
			// Signature algorithms
			&utls.SignatureAlgorithmsExtension{
				SupportedSignatureAlgorithms: []utls.SignatureScheme{
					utls.ECDSAWithP256AndSHA256,
					utls.ECDSAWithP384AndSHA384,
					utls.ECDSAWithP521AndSHA512,
					utls.PSSWithSHA256,
					utls.PSSWithSHA384,
					utls.PSSWithSHA512,
					utls.PKCS1WithSHA256,
					utls.PKCS1WithSHA384,
					utls.PKCS1WithSHA512,
				},
			},
			// PSK key exchange modes (for TLS 1.3 resumption)
			&utls.PSKKeyExchangeModesExtension{
				Modes: []uint8{utls.PskModeDHE},
			},
			// Key share for TLS 1.3
			&utls.KeyShareExtension{
				KeyShares: []utls.KeyShare{
					{Group: utls.X25519, Data: []byte{0x00}},
				},
			},
			// Extended Master Secret (for TLS 1.2)
			&utls.UtlsExtendedMasterSecretExtension{},
			// Status request (OCSP stapling)
			&utls.StatusRequestExtension{},
		},
		TLSVersMin: utls.VersionTLS12,
		TLSVersMax: utls.VersionTLS13,
	}
}

// GetClaudeCodeHelloSpec returns the ClientHelloSpec for Claude Code CLI
// Claude Code is a Node.js CLI application
// User-Agent: claude-cli/2.0.76 (external, cli)
func GetClaudeCodeHelloSpec() *utls.ClientHelloSpec {
	return &utls.ClientHelloSpec{
		CipherSuites: []uint16{
			utls.GREASE_PLACEHOLDER,
			utls.TLS_AES_128_GCM_SHA256,
			utls.TLS_AES_256_GCM_SHA384,
			utls.TLS_CHACHA20_POLY1305_SHA256,
			utls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			utls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			utls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			utls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			utls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			utls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
			utls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
			utls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			utls.TLS_RSA_WITH_AES_128_GCM_SHA256,
			utls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			utls.TLS_RSA_WITH_AES_128_CBC_SHA,
			utls.TLS_RSA_WITH_AES_256_CBC_SHA,
		},
		CompressionMethods: []byte{0x00},
		Extensions: []utls.TLSExtension{
			&utls.UtlsPreSharedKeyExtension{},
			&utls.SupportedVersionsExtension{
				Versions: []uint16{
					utls.GREASE_PLACEHOLDER,
					utls.VersionTLS13,
					utls.VersionTLS12,
				},
			},
			&utls.UtlsExtendedMasterSecretExtension{},
			&utls.RenegotiationInfoExtension{},
			&utls.SNIExtension{},
			&utls.StatusRequestExtension{},
			&utls.SupportedCurvesExtension{
				Curves: []utls.CurveID{
					utls.GREASE_PLACEHOLDER,
					utls.X25519,
					utls.CurveP256,
					utls.CurveP384,
				},
			},
			&utls.SupportedPointsExtension{
				SupportedPoints: []byte{0x00},
			},
			&utls.ALPNExtension{
				AlpnProtocols: []string{"h2", "http/1.1"},
			},
			&utls.SignatureAlgorithmsExtension{
				SupportedSignatureAlgorithms: []utls.SignatureScheme{
					utls.ECDSAWithP256AndSHA256,
					utls.ECDSAWithP384AndSHA384,
					utls.ECDSAWithP521AndSHA512,
					utls.PSSWithSHA256,
					utls.PSSWithSHA384,
					utls.PSSWithSHA512,
					utls.PKCS1WithSHA256,
					utls.PKCS1WithSHA384,
					utls.PKCS1WithSHA512,
				},
			},
			&utls.PSKKeyExchangeModesExtension{
				Modes: []uint8{utls.PskModeDHE},
			},
			&utls.KeyShareExtension{
				KeyShares: []utls.KeyShare{
					{Group: utls.GREASE_PLACEHOLDER, Data: []byte{0x00}},
					{Group: utls.X25519, Data: []byte{0x00}},
				},
			},
			&utls.UtlsPaddingExtension{
				GetPaddingLen: utls.BoringPaddingStyle,
			},
		},
		TLSVersMin: utls.VersionTLS12,
		TLSVersMax: utls.VersionTLS13,
	}
}

// GetCodexHelloSpec returns the ClientHelloSpec for OpenAI Codex CLI
// Codex CLI uses OpenAI's OAuth and API endpoints
func GetCodexHelloSpec() *utls.ClientHelloSpec {
	return &utls.ClientHelloSpec{
		CipherSuites: []uint16{
			utls.GREASE_PLACEHOLDER,
			utls.TLS_AES_128_GCM_SHA256,
			utls.TLS_AES_256_GCM_SHA384,
			utls.TLS_CHACHA20_POLY1305_SHA256,
			utls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			utls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			utls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			utls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			utls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			utls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
			utls.TLS_RSA_WITH_AES_128_GCM_SHA256,
			utls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		},
		CompressionMethods: []byte{0x00},
		Extensions: []utls.TLSExtension{
			&utls.UtlsPreSharedKeyExtension{},
			&utls.SupportedVersionsExtension{
				Versions: []uint16{
					utls.GREASE_PLACEHOLDER,
					utls.VersionTLS13,
					utls.VersionTLS12,
				},
			},
			&utls.SNIExtension{},
			&utls.StatusRequestExtension{},
			&utls.SupportedCurvesExtension{
				Curves: []utls.CurveID{
					utls.GREASE_PLACEHOLDER,
					utls.X25519,
					utls.CurveP256,
					utls.CurveP384,
				},
			},
			&utls.SupportedPointsExtension{
				SupportedPoints: []byte{0x00},
			},
			&utls.ALPNExtension{
				AlpnProtocols: []string{"h2", "http/1.1"},
			},
			&utls.SignatureAlgorithmsExtension{
				SupportedSignatureAlgorithms: []utls.SignatureScheme{
					utls.ECDSAWithP256AndSHA256,
					utls.PSSWithSHA256,
					utls.PSSWithSHA384,
					utls.PSSWithSHA512,
					utls.PKCS1WithSHA256,
					utls.PKCS1WithSHA384,
					utls.PKCS1WithSHA512,
				},
			},
			&utls.PSKKeyExchangeModesExtension{
				Modes: []uint8{utls.PskModeDHE},
			},
			&utls.KeyShareExtension{
				KeyShares: []utls.KeyShare{
					{Group: utls.GREASE_PLACEHOLDER, Data: []byte{0x00}},
					{Group: utls.X25519, Data: []byte{0x00}},
				},
			},
			&utls.UtlsPaddingExtension{
				GetPaddingLen: utls.BoringPaddingStyle,
			},
		},
		TLSVersMin: utls.VersionTLS12,
		TLSVersMax: utls.VersionTLS13,
	}
}

// GetGeminiCLIHelloSpec returns the ClientHelloSpec for Google Gemini CLI
// Gemini CLI uses Google's OAuth and API endpoints - prefers Chrome-like fingerprint
func GetGeminiCLIHelloSpec() *utls.ClientHelloSpec {
	return &utls.ClientHelloSpec{
		CipherSuites: []uint16{
			utls.GREASE_PLACEHOLDER,
			utls.TLS_AES_128_GCM_SHA256,
			utls.TLS_AES_256_GCM_SHA384,
			utls.TLS_CHACHA20_POLY1305_SHA256,
			utls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			utls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			utls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			utls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			utls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			utls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
			utls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
			utls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			utls.TLS_RSA_WITH_AES_128_GCM_SHA256,
			utls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			utls.TLS_RSA_WITH_AES_128_CBC_SHA,
			utls.TLS_RSA_WITH_AES_256_CBC_SHA,
		},
		CompressionMethods: []byte{0x00},
		Extensions: []utls.TLSExtension{
			&utls.UtlsPreSharedKeyExtension{},
			&utls.SupportedVersionsExtension{
				Versions: []uint16{
					utls.GREASE_PLACEHOLDER,
					utls.VersionTLS13,
					utls.VersionTLS12,
				},
			},
			&utls.UtlsExtendedMasterSecretExtension{},
			&utls.RenegotiationInfoExtension{},
			&utls.SNIExtension{},
			&utls.StatusRequestExtension{},
			&utls.SupportedCurvesExtension{
				Curves: []utls.CurveID{
					utls.GREASE_PLACEHOLDER,
					utls.X25519,
					utls.CurveP256,
					utls.CurveP384,
				},
			},
			&utls.SupportedPointsExtension{
				SupportedPoints: []byte{0x00},
			},
			&utls.ALPNExtension{
				AlpnProtocols: []string{"h2", "http/1.1"},
			},
			&utls.SignatureAlgorithmsExtension{
				SupportedSignatureAlgorithms: []utls.SignatureScheme{
					utls.ECDSAWithP256AndSHA256,
					utls.ECDSAWithP384AndSHA384,
					utls.ECDSAWithP521AndSHA512,
					utls.PSSWithSHA256,
					utls.PSSWithSHA384,
					utls.PSSWithSHA512,
					utls.PKCS1WithSHA256,
					utls.PKCS1WithSHA384,
					utls.PKCS1WithSHA512,
				},
			},
			&utls.PSKKeyExchangeModesExtension{
				Modes: []uint8{utls.PskModeDHE},
			},
			&utls.KeyShareExtension{
				KeyShares: []utls.KeyShare{
					{Group: utls.GREASE_PLACEHOLDER, Data: []byte{0x00}},
					{Group: utls.X25519, Data: []byte{0x00}},
				},
			},
			&utls.UtlsPaddingExtension{
				GetPaddingLen: utls.BoringPaddingStyle,
			},
		},
		TLSVersMin: utls.VersionTLS12,
		TLSVersMax: utls.VersionTLS13,
	}
}

// GetQwenCodeHelloSpec returns the ClientHelloSpec for Qwen Code CLI (通义千问)
// Qwen Code is a CLI application for Alibaba's Qwen AI service
func GetQwenCodeHelloSpec() *utls.ClientHelloSpec {
	return &utls.ClientHelloSpec{
		CipherSuites: []uint16{
			utls.GREASE_PLACEHOLDER,
			utls.TLS_AES_128_GCM_SHA256,
			utls.TLS_AES_256_GCM_SHA384,
			utls.TLS_CHACHA20_POLY1305_SHA256,
			utls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			utls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			utls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			utls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			utls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			utls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
			utls.TLS_RSA_WITH_AES_128_GCM_SHA256,
			utls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		},
		CompressionMethods: []byte{0x00},
		Extensions: []utls.TLSExtension{
			&utls.UtlsPreSharedKeyExtension{},
			&utls.SupportedVersionsExtension{
				Versions: []uint16{
					utls.GREASE_PLACEHOLDER,
					utls.VersionTLS13,
					utls.VersionTLS12,
				},
			},
			&utls.SNIExtension{},
			&utls.StatusRequestExtension{},
			&utls.SupportedCurvesExtension{
				Curves: []utls.CurveID{
					utls.GREASE_PLACEHOLDER,
					utls.X25519,
					utls.CurveP256,
					utls.CurveP384,
				},
			},
			&utls.SupportedPointsExtension{
				SupportedPoints: []byte{0x00},
			},
			&utls.ALPNExtension{
				AlpnProtocols: []string{"h2", "http/1.1"},
			},
			&utls.SignatureAlgorithmsExtension{
				SupportedSignatureAlgorithms: []utls.SignatureScheme{
					utls.ECDSAWithP256AndSHA256,
					utls.PSSWithSHA256,
					utls.PSSWithSHA384,
					utls.PSSWithSHA512,
					utls.PKCS1WithSHA256,
					utls.PKCS1WithSHA384,
					utls.PKCS1WithSHA512,
				},
			},
			&utls.PSKKeyExchangeModesExtension{
				Modes: []uint8{utls.PskModeDHE},
			},
			&utls.KeyShareExtension{
				KeyShares: []utls.KeyShare{
					{Group: utls.GREASE_PLACEHOLDER, Data: []byte{0x00}},
					{Group: utls.X25519, Data: []byte{0x00}},
				},
			},
			&utls.UtlsPaddingExtension{
				GetPaddingLen: utls.BoringPaddingStyle,
			},
		},
		TLSVersMin: utls.VersionTLS12,
		TLSVersMax: utls.VersionTLS13,
	}
}
