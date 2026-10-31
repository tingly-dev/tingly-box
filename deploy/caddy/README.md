# Caddy deployment

Replace `you.domain` in `Caddyfile` with the public DNS name and keep the TB
HTTP listener reachable at `localhost:12580` (or update both proxy targets).
Caddy serves HTTPS and HTTP/3 on the same public port, 443: expose **TCP 443 and
UDP 443**. TCP 80 is used by automatic HTTPS redirects and may be needed for
HTTP-01 certificate issuance. Existing network or container firewall rules must
publish both 443 protocols.

The deployment forwards `/tingly/*` to the existing TB HTTP listener for model
requests and same-path HEAD capability probes. TB handles the probe and identifies
itself; Caddy only forwards it. Point a
local TB peer at the authenticated `https://` URL. Plain `http://` peer URLs do
not automatically upgrade. Existing `/mirror/*` and `/hi` behavior is retained.
Caddy serves SSE without response buffering using its default event-stream
handling; it can propagate client cancellation to TB.

Caddy obtains and manages certificates for a public DNS name by default. For a
private or test deployment, configure a certificate trusted by the connecting
TB or use Caddy's internal CA and explicitly trust its root. Never disable TLS
verification. Caddy owns HTTPS and QUIC; TB remains an HTTP backend, so no TB
certificate flags or extra TB HTTPS port are needed. A failed request already
sent over H3 is not silently replayed; subsequent requests may fall back to TCP.

Validate before deploying:

```sh
caddy fmt --diff deploy/caddy/Caddyfile
caddy adapt --config deploy/caddy/Caddyfile --validate
CADDY_BIN=/absolute/path/to/caddy go test -tags=e2e ./internal/protocoltest \
  -run '^TestCaddyDeploymentConfigHTTP3$' -v
```

The default auto-HTTPS redirect from TCP 80 is a redirect, not transparent
plaintext HTTP support on TCP 443; clients following it must trust the certificate.
