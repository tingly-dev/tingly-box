#!/usr/bin/env bash
# Smoke test for deploy/caddy/Caddyfile (model API only) and
# deploy/caddy/Caddyfile.full (whole service): runs both in the official caddy
# image in front of a stub gateway and checks routing, compression, the
# pass-through of the gateway's own cache/encoding headers, HTTP/2 and SSE.
#
#   deploy/caddy/test.sh            # needs docker, python3, curl
#
# Never touches ~/.tingly-box: the stub is a small python server and Caddy gets
# its own HOME inside the container.
set -euo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
WORK=$(mktemp -d "${TMPDIR:-/tmp}/caddy-test.XXXXXX")
NAME=tingly-caddy-test-$$
STUB_PORT=12580   # fixed: the Caddyfiles proxy to localhost:12580
if (exec 3<>"/dev/tcp/127.0.0.1/$STUB_PORT") 2>/dev/null; then
    echo "port $STUB_PORT is in use (a running gateway?): stop it first" >&2
    exit 2
fi
free_port() { python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])'; }
PORT_MODEL=$(free_port)   # Caddyfile, plain http
PORT_FULL=$(free_port)    # Caddyfile.full, https (Caddy's internal CA) to see h2

cleanup() {
    docker rm -f "$NAME-m" "$NAME-f" >/dev/null 2>&1 || true
    [ -n "${STUB_PID:-}" ] && kill "$STUB_PID" 2>/dev/null || true
    rm -rf "$WORK"
}
trap cleanup EXIT

# --- stub gateway. It behaves like the real one where it matters here:
# /assets/* comes precompressed (Content-Encoding set) with immutable caching,
# an unknown asset is a 404, everything else is identity-encoded.
cat >"$WORK/stub.py" <<'PY'
import gzip, http.server, json, sys, time
port = int(sys.argv[1])
JS = ("export const a = 'tingly box';\n" * 5000).encode()
JS_GZ = gzip.compress(JS, 9)
HTML = ("<script src='/assets/index-AbCd1234.js'></script>" * 40).encode()
API = json.dumps({"models": ["x" * 40] * 50}).encode()
class H(http.server.BaseHTTPRequestHandler):
    def log_message(self, *a): pass
    def send(self, code, ctype, body, extra=()):
        self.send_response(code); self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(body)))
        for k, v in extra: self.send_header(k, v)
        self.end_headers(); self.wfile.write(body)
    def do_GET(self):
        p = self.path
        if p == "/assets/index-AbCd1234.js":
            hdr = [("Cache-Control", "public, max-age=31536000, immutable"), ("Vary", "Accept-Encoding")]
            if "gzip" in self.headers.get("Accept-Encoding", ""):
                return self.send(200, "text/javascript", JS_GZ, hdr + [("Content-Encoding", "gzip")])
            return self.send(200, "text/javascript", JS, hdr)
        if p.startswith("/assets/"):
            return self.send(404, "text/plain; charset=utf-8", b"not found", [("Cache-Control", "no-cache")])
        if p == "/raw.js":
            return self.send(200, "text/javascript", JS)
        if p.startswith("/tingly/json") or p.startswith("/api/v1/"):
            return self.send(200, "application/json", API)
        if p.startswith("/tingly/sse"):
            self.send_response(200); self.send_header("Content-Type", "text/event-stream")
            self.send_header("Transfer-Encoding", "chunked"); self.end_headers()
            for i in range(2):
                c = ("data: tick %d\n\n" % i).encode()
                self.wfile.write(b"%x\r\n%s\r\n" % (len(c), c)); self.wfile.flush(); time.sleep(0.6)
            self.wfile.write(b"0\r\n\r\n"); return
        if p.startswith("/tingly/jsonstream"):
            # a JSON body written in small flushed pieces (not SSE)
            self.send_response(200); self.send_header("Content-Type", "application/json")
            self.send_header("Transfer-Encoding", "chunked"); self.end_headers()
            for i in range(2):
                c = ('{"part": %d}\n' % i).encode()
                self.wfile.write(b"%x\r\n%s\r\n" % (len(c), c)); self.wfile.flush(); time.sleep(0.6)
            self.wfile.write(b"0\r\n\r\n"); return
        # SPA fallback
        self.send(200, "text/html; charset=utf-8", HTML, [("Cache-Control", "no-cache")])
http.server.ThreadingHTTPServer(("127.0.0.1", port), H).serve_forever()
PY
python3 "$WORK/stub.py" "$STUB_PORT" &
STUB_PID=$!

# --- test copies of the Caddyfiles: the real host -> a local port
adapt() { # $1 file  $2 site address
    sed -e "s#^you\.domain {#$2 {#" -e '0,/^{$/s//{\n\tadmin off/' "$HERE/$1"
}
adapt Caddyfile "http://localhost:$PORT_MODEL" | sed 's#^\t\tprotocols h1 h2 h3#\t\tprotocols h1 h2#' >"$WORK/model.caddy"
adapt Caddyfile.full "localhost:$PORT_FULL" >"$WORK/full.caddy"
for f in model full; do
    docker run --rm -v "$WORK:/w:ro" caddy:2 caddy validate --config "/w/$f.caddy" --adapter caddyfile >/dev/null 2>&1 \
        || { echo "FAIL: $f config does not validate"; exit 1; }
done
# the unmodified files too (real host names, h3): validate only
for f in Caddyfile Caddyfile.full; do
    docker run --rm -v "$HERE:/w:ro" caddy:2 sh -c "cp /w/$f /tmp/c && caddy validate --config /tmp/c --adapter caddyfile" >/dev/null 2>&1 \
        || { echo "FAIL: $f does not validate"; exit 1; }
done

run() { docker run -d --name "$1" --network host -e HOME=/tmp/h -v "$WORK:/w:ro" caddy:2 \
        caddy run --config "/w/$2" --adapter caddyfile >/dev/null; }
run "$NAME-m" model.caddy
run "$NAME-f" full.caddy
for _ in $(seq 60); do
    curl -s -o /dev/null "http://localhost:$PORT_MODEL/hi" && curl -sk -o /dev/null "https://localhost:$PORT_FULL/hi" && break
    sleep 0.25
done

fails=0
check() { # name got want
    if [ "$2" = "$3" ]; then echo "ok   $1"; else echo "FAIL $1: got '$2' want '$3'"; fails=$((fails + 1)); fi
}
hdr() { curl -sk -o /dev/null -D - "${@:2}" | tr -d '\r' | grep -i "^$1:" | cut -d' ' -f2- || true; }
code() { curl -sk -o /dev/null -w '%{http_code}' "$@"; }
wire() { curl -sk -o /dev/null -w '%{size_download}' "$@"; }
GZ=(-H 'Accept-Encoding: gzip')

echo "# model API only (Caddyfile)"
M=http://localhost:$PORT_MODEL
check "json compressed" "$(hdr content-encoding "${GZ[@]}" $M/tingly/json)" "gzip"
check "mirror json compressed" "$(hdr content-encoding "${GZ[@]}" $M/mirror/json)" "gzip"
# compression is negotiated: the client decides, and anything it cannot take is sent as is
check "no Accept-Encoding -> plain" "$(hdr content-encoding $M/tingly/json)" ""
check "identity only -> plain" "$(hdr content-encoding -H 'Accept-Encoding: identity' $M/tingly/json)" ""
check "only br (not offered by Caddy) -> plain" "$(hdr content-encoding -H 'Accept-Encoding: br' $M/tingly/json)" ""
check "gzip;q=0 -> plain" "$(hdr content-encoding -H 'Accept-Encoding: gzip;q=0' $M/tingly/json)" ""
check "zstd client gets zstd" "$(hdr content-encoding -H 'Accept-Encoding: zstd' $M/tingly/json)" "zstd"
check "gzip, br falls back to gzip" "$(hdr content-encoding -H 'Accept-Encoding: gzip, br' $M/tingly/json)" "gzip"
check "plain response is the full body" "$(curl -s $M/tingly/json | wc -c | tr -d ' ')" "$(curl -s --compressed $M/tingly/json | wc -c | tr -d ' ')"
check "Vary says it depends on the request" "$(hdr vary "${GZ[@]}" $M/tingly/json)" "Accept-Encoding"
check "sse not compressed" "$(hdr content-encoding "${GZ[@]}" --max-time 3 $M/tingly/sse)" ""
check "health check" "$(code $M/hi)" "200"
for p in / /api/v1/info /assets/index-AbCd1234.js /some/route; do
    check "$p is forbidden" "$(code $M$p)" "403"
done

echo "# whole service (Caddyfile.full)"
F=https://localhost:$PORT_FULL
check "http/2" "$(curl -sk --http2 -o /dev/null -w '%{http_version}' $F/hi)" "2"
check "precompressed asset passes through encoded once" "$(hdr content-encoding "${GZ[@]}" $F/assets/index-AbCd1234.js)" "gzip"
[ "$(wire "${GZ[@]}" $F/assets/index-AbCd1234.js)" -lt 2000 ] && echo "ok   asset small on the wire" || { echo "FAIL asset size"; fails=$((fails + 1)); }
check "asset decodes to the original (not double-encoded)" "$(curl -sk --compressed $F/assets/index-AbCd1234.js | wc -c | tr -d ' ')" "155000"
check "gateway cache policy kept" "$(hdr cache-control "${GZ[@]}" $F/assets/index-AbCd1234.js)" "public, max-age=31536000, immutable"
check "client without gzip still served" "$(code $F/assets/index-AbCd1234.js)" "200"
check "missing asset is a 404" "$(code $F/assets/gone-ZZZZZZZZ.js)" "404"
check "404 not cacheable as immutable" "$(hdr cache-control $F/assets/gone-ZZZZZZZZ.js)" "no-cache"
check "html compressed" "$(hdr content-encoding "${GZ[@]}" $F/some/route)" "gzip"
check "html keeps no-cache" "$(hdr cache-control "${GZ[@]}" $F/some/route)" "no-cache"
check "management api json compressed" "$(hdr content-encoding "${GZ[@]}" $F/api/v1/info)" "gzip"
check "model api json compressed" "$(hdr content-encoding "${GZ[@]}" $F/tingly/json)" "gzip"
check "mirror json compressed" "$(hdr content-encoding "${GZ[@]}" $F/mirror/json)" "gzip"
check "unencoded script compressed" "$(hdr content-encoding "${GZ[@]}" $F/raw.js)" "gzip"
check "sse not compressed" "$(hdr content-encoding "${GZ[@]}" --max-time 3 $F/tingly/sse)" ""
check "health check" "$(code $F/hi)" "200"

# an SSE event, or a flushed piece of a streamed JSON body, must reach the
# client while the response is still open
for url in $M/tingly/sse $F/tingly/sse $M/tingly/jsonstream $F/tingly/jsonstream; do
    first=$(curl -sk -N "${GZ[@]}" --max-time 3 "$url" | while IFS= read -r l; do [ -n "$l" ] && { date +%s%N; break; }; done || true)
    now=$(date +%s%N)
    [ $(((now - first) / 1000000)) -gt 400 ] && echo "ok   first piece delivered before the response ends ($url)" || { echo "FAIL buffered ($url)"; fails=$((fails + 1)); }
done

[ "$fails" -eq 0 ] && echo "all passed" || { echo "$fails failed"; exit 1; }
