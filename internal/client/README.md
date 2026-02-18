# Client Package - TLS Fingerprint Spoofing

## Overview

This package provides TLS fingerprint spoofing capability to help API requests avoid detection by services that perform TLS fingerprint checks.

## Key Concepts

### User-Agent vs ClientHelloSpec

**These are different things at different network layers:**

| Concept | Purpose | Layer |
|---------|---------|-------|
| **User-Agent** | HTTP header identifying client software | HTTP (L7) |
| **ClientHelloID** | uTLS library internal identifier for selecting fingerprint preset | Library internal |
| **ClientHelloSpec** | Actual TLS handshake data structure | TLS (L4) |

### JA3 Fingerprint

JA3 is a hash calculated from the **TLS ClientHello** message, including:
- TLS Version
- Cipher Suites
- TLS Extensions
- Elliptic Curves
- Point Formats

```
┌─────────────────────────────────────────────────────────┐
│                    Network Stack                         │
├─────────────────────────────────────────────────────────┤
│                                                          │
│   TLS Layer (L4)              HTTP Layer (L7)           │
│        │                           │                     │
│        ▼                           ▼                     │
│   ClientHelloSpec            User-Agent Header          │
│   (JA3 fingerprint)          (HTTP identity)            │
│        │                           │                     │
│        │     These are INDEPENDENT │                     │
│        │     and must be configured│ separately          │
│        ▼                           ▼                     │
│   tls_fingerprint_spec.go    http.go (hooks)            │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

## File Structure

```
internal/client/
├── tls_fingerprint.go        # TLSDialer implementation
├── tls_fingerprint_spec.go   # ClientHelloSpec definitions for each client
├── http.go                   # HTTP client factory with User-Agent hooks
├── transport_pool.go         # Transport pooling with fingerprint support
└── google.go, openai.go...   # Client implementations
```

## Configuration

### Provider Config

```json
{
  "name": "antigravity-provider",
  "tls_fingerprint": "antigravity"
}
```

### Supported Fingerprints

| Fingerprint | Client | Description |
|-------------|--------|-------------|
| `antigravity` | Antigravity | Windows desktop app |
| `claude_code` | Claude Code | Node.js CLI |
| `codex` | Codex | OpenAI CLI |
| `gemini_cli` | Gemini CLI | Google CLI |
| `qwen_code` | Qwen Code | 通义千问 CLI |
| `chrome` | Chrome | Generic Chrome browser |
| `firefox` | Firefox | Generic Firefox browser |
| `safari` | Safari | Generic Safari browser |
| `ios` | iOS | iOS Safari |
| `android` | Android | Android OkHttp |

## How to Customize Fingerprints

### Step 1: Capture Real TLS Fingerprint

Use Wireshark or similar tool to capture TLS ClientHello from the real client:

```bash
# Filter for TLS ClientHello
tls.handshake.type == 1
```

### Step 2: Update ClientHelloSpec

Edit `tls_fingerprint_spec.go` to match the captured data:

```go
func GetAntigravityHelloSpec() *utls.ClientHelloSpec {
    return &utls.ClientHelloSpec{
        CipherSuites: []uint16{
            // Match captured cipher suites
            utls.TLS_AES_128_GCM_SHA256,
            // ...
        },
        Extensions: []utls.TLSExtension{
            // Match captured extensions order
            &utls.SNIExtension{},
            &utls.ALPNExtension{AlpnProtocols: []string{"h2", "http/1.1"}},
            // ...
        },
    }
}
```

### Step 3: User-Agent is Separate

User-Agent is configured in HTTP hooks (`http.go`):

```go
func antigravityHook(req *http.Request) error {
    req.Header.Set("User-Agent", "antigravity/1.11.5 windows/amd64")
    // ...
}
```

## Integration Points

1. **transport_pool.go** - Creates transports with TLS fingerprint support
2. **http.go** - `CreateHTTPClientForProvider()` uses fingerprint from provider config
3. **google.go, openai.go, anthropic.go** - Pass fingerprint to HTTP client creation

## Dependencies

- `github.com/refraction-networking/utls` - TLS fingerprint spoofing library
