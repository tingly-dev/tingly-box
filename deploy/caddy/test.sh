#!/usr/bin/env bash
# Smoke test for deploy/caddy/Caddyfile: runs it in the official caddy image
# against a stub gateway and checks the transfer-cost headers.
#
#   deploy/caddy/test.sh            # needs docker, python3, node, curl
#
# Never touches ~/.tingly-box: the stub is a ~30 line python server and Caddy
# gets its own HOME inside the container.
set -euo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
WORK=$(mktemp -d "${TMPDIR:-/tmp}/caddy-test.XXXXXX")
NAME=tingly-caddy-test-$$
STUB_PORT=12580   # fixed: the Caddyfile proxies to localhost:12580
if (exec 3<>"/dev/tcp/127.0.0.1/$STUB_PORT") 2>/dev/null; then
    echo "port $STUB_PORT is in use (a running gateway?): stop it first" >&2
    exit 2
fi
free_port() { python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])'; }
PORT_API=$(free_port)        # you.domain
PORT_UI=$(free_port)         # ui.you.domain, assets proxied to the gateway
PORT_API_PRE=$(free_port)    # same, second instance
PORT_UI_PRE=$(free_port)     # ui.you.domain, assets from TINGLY_WEB_ROOT

cleanup() {
    docker rm -f "$NAME-a" "$NAME-b" >/dev/null 2>&1 || true
    [ -n "${STUB_PID:-}" ] && kill "$STUB_PID" 2>/dev/null || true
    rm -rf "$WORK"
}
trap cleanup EXIT

# --- fake build: hashed + unhashed assets, big enough to be worth compressing
mkdir -p "$WORK/dist/assets"
python3 - "$WORK/dist" <<'PY'
import sys, pathlib
d = pathlib.Path(sys.argv[1])
(d / "index.html").write_text('<script src="/assets/index-AbCd1234.js"></script>' * 20)
(d / "assets/index-AbCd1234.js").write_text("export const a = 'tingly box';\n" * 5000)
(d / "assets/icon.svg").write_text("<svg xmlns='http://www.w3.org/2000/svg'>" + "<g/>" * 300 + "</svg>")
PY
node "$ROOT/frontend/scripts/precompress.mjs" "$WORK/dist" "$WORK/deploy-web" >/dev/null

# --- stub gateway: /assets from dist, JSON API, SSE, SPA fallback = index.html 200
cat >"$WORK/stub.py" <<'PY'
import http.server, sys, time, json, pathlib
dist = pathlib.Path(sys.argv[1]); port = int(sys.argv[2])
class H(http.server.BaseHTTPRequestHandler):
    def log_message(self, *a): pass
    def send(self, code, ctype, body, extra=()):
        self.send_response(code); self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(body)))
        for k, v in extra: self.send_header(k, v)
        self.end_headers(); self.wfile.write(body)
    def do_GET(self):
        p = self.path
        if p.startswith("/assets/") and (dist / p[1:]).is_file():
            f = dist / p[1:]
            return self.send(200, "image/svg+xml" if p.endswith(".svg") else "text/javascript", f.read_bytes())
        if p.startswith("/tingly/json"):
            return self.send(200, "application/json", json.dumps({"m": ["x" * 40] * 50}).encode())
        if p.startswith("/tingly/sse"):
            self.send_response(200); self.send_header("Content-Type", "text/event-stream")
            self.send_header("Transfer-Encoding", "chunked"); self.end_headers()
            for i in range(2):
                c = ("data: tick %d\n\n" % i).encode()
                self.wfile.write(b"%x\r\n%s\r\n" % (len(c), c)); self.wfile.flush(); time.sleep(0.6)
            self.wfile.write(b"0\r\n\r\n"); return
        # SPA fallback, like UseWebStaticEndpoints' NoRoute
        self.send(200, "text/html; charset=utf-8", (dist / "index.html").read_bytes(),
                  [("Cache-Control", "no-cache, no-store, must-revalidate")])
http.server.ThreadingHTTPServer(("127.0.0.1", port), H).serve_forever()
PY
python3 "$WORK/stub.py" "$WORK/dist" "$STUB_PORT" &
STUB_PID=$!

# --- test copy of the Caddyfile: real hosts -> local plain-HTTP ports
adapt() { # $1 api port  $2 ui port
    sed -e "s#^ui\.you\.domain {#http://localhost:$2 {#" \
        -e "s#^you\.domain {#http://localhost:$1 {#" \
        -e 's#^\t\tprotocols h1 h2 h3#\t\tprotocols h1 h2#' \
        -e '0,/^{$/s//{\n\tadmin off/' "$HERE/Caddyfile"
}
adapt $PORT_API $PORT_UI >"$WORK/a.caddy"
adapt $PORT_API_PRE $PORT_UI_PRE >"$WORK/b.caddy"
docker run --rm -v "$WORK:/w:ro" caddy:2 caddy validate --config /w/a.caddy --adapter caddyfile >/dev/null 2>&1 \
    || { echo "FAIL: Caddyfile does not validate"; exit 1; }

run() { docker run -d --name "$1" --network host -e HOME=/tmp/h "${@:3}" -v "$WORK:/w:ro" caddy:2 \
        caddy run --config "/w/$2" --adapter caddyfile >/dev/null; }
run "$NAME-a" a.caddy -e TINGLY_WEB_ROOT=/nonexistent
run "$NAME-b" b.caddy -e TINGLY_WEB_ROOT=/w/deploy-web
for _ in $(seq 40); do curl -s -o /dev/null "localhost:$PORT_UI/" && curl -s -o /dev/null "localhost:$PORT_UI_PRE/" && break; sleep 0.25; done

fails=0
hdr() { curl -s -o /dev/null -D - "${@:2}" | tr -d '\r' | grep -i "^$1:" | cut -d' ' -f2- || true; }
size() { curl -s -o /dev/null -w '%{size_download}' "$@"; }
check() { # name got want
    if [ "$2" = "$3" ]; then echo "ok   $1"; else echo "FAIL $1: got '$2' want '$3'"; fails=$((fails + 1)); fi
}
JS=/assets/index-AbCd1234.js

for port in $PORT_UI $PORT_UI_PRE; do
    echo "# ui host :$port"
    check "hashed asset is immutable" "$(hdr cache-control localhost:$port$JS)" "public, max-age=31536000, immutable"
    check "unhashed asset has short ttl" "$(hdr cache-control localhost:$port/assets/icon.svg)" "public, max-age=3600"
    check "gzip negotiated" "$(hdr content-encoding -H 'Accept-Encoding: gzip' localhost:$port$JS)" "gzip"
    check "zstd negotiated" "$(hdr content-encoding -H 'Accept-Encoding: zstd, gzip' localhost:$port$JS)" "zstd"
    [ "$(size -H 'Accept-Encoding: gzip' localhost:$port$JS)" -lt 2000 ] && echo "ok   compressed size small" || { echo "FAIL compressed size"; fails=$((fails + 1)); }
    check "missing chunk is a 404, not index.html" "$(curl -s -o /dev/null -w '%{http_code}' localhost:$port/assets/gone-ZZZZZZZZ.js)" "404"
    check "404 not cacheable as immutable" "$(hdr cache-control localhost:$port/assets/gone-ZZZZZZZZ.js)" ""
    check "SPA route keeps gateway no-store" "$(hdr cache-control localhost:$port/some/route)" "no-cache, no-store, must-revalidate"
done
echo "# precompressed (TINGLY_WEB_ROOT)"
check "brotli served from .br" "$(hdr content-encoding -H 'Accept-Encoding: br, gzip' localhost:$PORT_UI_PRE$JS)" "br"
check "etag present" "$([ -n "$(hdr etag -H 'Accept-Encoding: br' localhost:$PORT_UI_PRE$JS)" ] && echo yes)" "yes"
ETAG=$(hdr etag -H 'Accept-Encoding: br' localhost:$PORT_UI_PRE$JS)
check "conditional request -> 304" "$(curl -s -o /dev/null -w '%{http_code}' -H 'Accept-Encoding: br' -H "If-None-Match: $ETAG" localhost:$PORT_UI_PRE$JS)" "304"
echo "# api host :$PORT_API"
check "json compressed" "$(hdr content-encoding -H 'Accept-Encoding: gzip' localhost:$PORT_API/tingly/json)" "gzip"
check "mirror json compressed" "$(hdr content-encoding -H 'Accept-Encoding: gzip' localhost:$PORT_API/mirror/json)" "gzip"
check "sse not compressed" "$(hdr content-encoding -H 'Accept-Encoding: gzip' --max-time 3 localhost:$PORT_API/tingly/sse)" ""
first=$(curl -s -N -H 'Accept-Encoding: gzip' --max-time 3 localhost:$PORT_API/tingly/sse | while IFS= read -r l; do [ -n "$l" ] && { date +%s%N; break; }; done || true)
now=$(date +%s%N)
[ $(((now - first) / 1000000)) -gt 400 ] && echo "ok   sse event reaches client before stream ends" || { echo "FAIL sse buffered"; fails=$((fails + 1)); }
check "everything else still forbidden" "$(curl -s -o /dev/null -w '%{http_code}' localhost:$PORT_API/assets/x.js)" "403"

[ "$fails" -eq 0 ] && echo "all passed" || { echo "$fails failed"; exit 1; }
