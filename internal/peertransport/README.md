# TB peer HTTP/3

A local TB can use HTTP/3 when its configured HTTPS peer advertises it and a
side-effect-free capability probe succeeds. Existing agents keep their normal
HTTP connection to the local TB. On failure, subsequent requests use the
existing TCP path; an already submitted request is never replayed by this
transport selector.

The cloud TB exposes the probe on its existing HTTP listener. TLS, certificates,
TCP/UDP listeners and HTTP/3 termination belong to the deployment proxy. For
example, Caddy can listen on the same public port for HTTPS/TCP and QUIC/UDP,
and proxy both to the cloud TB's existing HTTP port. The probe uses the same URL as the model request with a dedicated HEAD header,
so existing proxy routes and path prefixes are preserved. Configure the local
TB's peer URL as the proxy's trusted HTTPS URL. Plain `http://` URLs keep their
existing transport; they are not automatically upgraded.

The first request uses TCP. A verified HTTPS response advertising same-origin,
same-port HTTP/3 starts a background HEAD probe. The cloud TB answers a
`Tingly-Transport-Probe: tb-peer-v1` request with `Tingly-Transport: tb-peer-v1`;
this identifies the TB peer, not direct HTTP/3 support by the TB listener.
The probe must also reach TB over HTTP/3 through the deployment. Success enables HTTP/3 for
later requests; failure enters a bounded cooldown. Explicit HTTP/SOCKS proxies
retain their configured path. Set `TINGLY_HTTP3=off` to disable discovery for
new provider clients. Generic OpenAI and Anthropic provider paths are covered;
OAuth-specific transports and WebSockets retain their existing paths.

Self-signed certificates require explicit trust at the client or proxy; TLS
verification remains enabled. A standard Caddy TLS listener does not transparently
serve plaintext HTTP on its HTTPS port. Its optional HTTP redirect requires
clients to follow the redirect and trust the certificate. If Caddy is configured
to use HTTP/3 to its backend, Caddy's HTTP/3-only upstream does not automatically
fall back to HTTP/2. Network performance gains have not yet been benchmarked.
