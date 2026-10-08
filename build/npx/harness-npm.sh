#!/usr/bin/env bash
# harness-npm: rehearse the whole npm release against a local registry, from the
# current source, under a virtual version nobody has ever published.
#
#   source ──build──▶ binaries ──zip──▶ platform packages ─┐
#   build/npx/{tingly-box,tingly-box-gui} ──CI steps──▶ shims ─┴─publish─▶ local
#   registry (Verdaccio) ──npm install -g / npx / tb gui──▶ run, assert
#
# Why not a real release: the published versions are what is (or may be) broken,
# so testing against them says nothing about this code. The version here is
# virtual (>= 900.0.0, default 999.0.0), the registry is local and the binaries
# are built from the checkout, so every run exercises exactly this tree and can
# never collide with, or publish to, the real registry. Design: .design/harness-npm.md.
#
# Usage: build/npx/harness-npm.sh [--version 999.0.0] [--no-gui-launch] [--keep]
#   --no-gui-launch   skip starting the GUI (no display needed)
#   --keep            keep the work directory (logs, registry storage) for inspection
# Runs on Linux, macOS and Windows (Git Bash). Needs node, npm, python3 (or python),
# curl and go. The GUI additionally needs: Linux: libgtk-3-dev libwebkit2gtk-4.1-dev,
# Xvfb and dbus-launch to start it; macOS (Apple Silicon) and Windows (x64): go-task
# and wails3, plus the frontend dependencies unless frontend/dist is already built and
# FRONTEND_PREBUILT=true (what .github/workflows/harness-npm.yml does). The binaries
# embed whatever internal/web/dist holds: the placeholder page unless it was built.
# macOS starts the app through `open`, which ignores this shell's environment, so the
# GUI launch there uses the real home directory and only runs when CI=true (or
# HARNESS_ALLOW_REAL_HOME=1); a developer's own ~/.tingly-box is never touched.
set -uo pipefail

die() { echo "❌ $*" >&2; exit 2; }

# ----------------------------------------------------------------------- host
case "$(uname -s)" in
	Linux) HOST=linux ;;
	Darwin) HOST=darwin ;;
	MINGW*|MSYS*|CYGWIN*) HOST=win32 ;;
	*) die "unsupported host: $(uname -s)" ;;
esac
case "$(uname -m)" in
	x86_64|AMD64) CPU=amd64 ;;
	aarch64|arm64|ARM64) CPU=arm64 ;;
	*) die "unsupported CPU: $(uname -m)" ;;
esac
EXE=""; ZOS="$HOST"   # release zip names: tingly-box-<linux|macos|windows>-<amd64|arm64>.zip
case "$HOST" in darwin) ZOS=macos ;; win32) ZOS=windows; EXE=.exe ;; esac

# Under Git Bash, mktemp and pwd give MSYS paths (/tmp/…, /d/a/…) that native node cannot
# resolve when they end up inside a string; the mixed form (C:/…) works for both sides.
winpath() { if [ "$HOST" = win32 ] && command -v cygpath >/dev/null 2>&1; then cygpath -m "$1"; else printf '%s' "$1"; fi; }

SCRIPT_DIR="$(winpath "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)")"
REPO_ROOT="$(winpath "$(cd "$SCRIPT_DIR/../.." && pwd)")"
VERSION="999.0.0"; GUI_LAUNCH=1; KEEP=0
while [ $# -gt 0 ]; do
	case "$1" in
		--version) VERSION="$2"; shift 2 ;;
		--no-gui-launch) GUI_LAUNCH=0; shift ;;
		--keep) KEEP=1; shift ;;
		-h|--help) sed -n 2,23p "$0"; exit 0 ;;
		*) echo "unknown argument: $1" >&2; exit 2 ;;
	esac
done

# The guard that makes the rehearsal honest: a version that exists upstream would be tested
# against artifacts that are not this tree's.
MAJOR="${VERSION%%.*}"
case "$MAJOR" in ''|*[!0-9]*) die "bad version: $VERSION" ;; esac
[ "$MAJOR" -ge 900 ] || die "version $VERSION looks like a real release; use a virtual one (>= 900.0.0), e.g. 999.0.0"
TAG="v$VERSION"

# python3 is a Microsoft Store stub on some Windows images: accept a candidate only if it runs.
PY=""
for c in python3 python; do
	if command -v "$c" >/dev/null 2>&1 && "$c" -c 'import sys; sys.exit(0 if sys.version_info[0] == 3 else 1)' >/dev/null 2>&1; then PY="$c"; break; fi
done
for tool in node npm curl go; do command -v "$tool" >/dev/null 2>&1 || die "missing tool: $tool"; done
[ -n "$PY" ] || die "missing tool: python3"
export PY

WORK="$(winpath "$(mktemp -d)")"
[ -n "$WORK" ] && [ -d "$WORK" ] || die "mktemp failed"
REG_PID=""
PASS=0; FAIL=0
pass() { echo "✅ $1"; PASS=$((PASS + 1)); }
fail() { echo "❌ $1"; FAIL=$((FAIL + 1)); }
cleanup() {
	# `|| true`: under `set -e` a failing command in an EXIT trap turns a passing run into exit 1
	[ -n "$REG_PID" ] && kill "$REG_PID" 2>/dev/null || true
	if [ "$KEEP" -eq 1 ]; then echo "work dir kept: $WORK"; else rm -rf "$WORK"; fi
}
trap cleanup EXIT

REAL_REGISTRY="https://registry.npmjs.org/"
LOG="$WORK/logs"; mkdir -p "$LOG"
echo "harness-npm on $HOST-$CPU, virtual version $TAG"

# ---------------------------------------------------------------- 1. artifacts
echo "==> [1] artifacts for $TAG"
ZIPS="$WORK/zips"; mkdir -p "$ZIPS" "$WORK/bin"
mkzip() { # <zip> <file> <name inside>: one file at the zip's top level, executable
	"$PY" - "$1" "$2" "$3" <<'EOF'
import sys, zipfile
out, src, name = sys.argv[1:]
zi = zipfile.ZipInfo(name); zi.external_attr = 0o755 << 16; zi.compress_type = zipfile.ZIP_DEFLATED
with zipfile.ZipFile(out, "w") as z:
    z.writestr(zi, open(src, "rb").read())
EOF
}

if (cd "$REPO_ROOT" && CGO_ENABLED=1 go build -tags sqlite_omit_load_extension -trimpath \
	-ldflags "-s -w -X main.version=$TAG" -o "$WORK/bin/tingly-box$EXE" ./cli/tingly-box) > "$LOG/build-cli.log" 2>&1; then
	mkzip "$ZIPS/tingly-box-$ZOS-$CPU.zip" "$WORK/bin/tingly-box$EXE" "tingly-box$EXE"
	pass "built the CLI from source ($(du -h "$WORK/bin/tingly-box$EXE" | cut -f1)), stamped $TAG"
else
	fail "CLI build failed (see $LOG/build-cli.log)"; tail -5 "$LOG/build-cli.log"; exit 1
fi

# The desktop app. Linux builds the GTK3 variant with plain go build (the zip the npm package
# ships). macOS and Windows use the same go-task targets as release-gui.yml, so the bundle,
# plist and Windows subsystem flags are the release's, not a copy of them.
build_gui_task() { # <task args...>: needs go-task + wails3 (+ the frontend deps)
	if ! command -v task >/dev/null 2>&1 || ! command -v wails3 >/dev/null 2>&1; then
		echo "ℹ️  go-task / wails3 not found: no GUI artifact, GUI steps will be skipped (see release-gui.yml for the setup)"
		return 1
	fi
	(cd "$REPO_ROOT" && task "$@" VERSION="$TAG") > "$LOG/build-gui.log" 2>&1 || { fail "GUI build failed (see $LOG/build-gui.log)"; tail -8 "$LOG/build-gui.log"; return 1; }
}
case "$HOST" in
	linux)
		if pkg-config --exists gtk+-3.0 webkit2gtk-4.1 2>/dev/null; then
			if (cd "$REPO_ROOT/gui/wails3" && CGO_ENABLED=1 go build -tags "production gtk3" -trimpath -buildvcs=false \
				-ldflags "-w -s -X main.version=$TAG" -o "$WORK/bin/tingly-box-gui" ./) > "$LOG/build-gui.log" 2>&1; then
				mkzip "$ZIPS/tingly-box-gui-linux-$CPU.zip" "$WORK/bin/tingly-box-gui" tingly-box-gui
				pass "built the GTK3 GUI from source ($(du -h "$WORK/bin/tingly-box-gui" | cut -f1)); UI is internal/web/dist's (placeholder unless the frontend was built)"
			else
				fail "GUI build failed (see $LOG/build-gui.log)"; tail -5 "$LOG/build-gui.log"
			fi
		else
			echo "ℹ️  GTK3 / WebKitGTK 4.1 dev packages not found: no GUI artifact, GUI steps will be skipped"
		fi ;;
	darwin)
		if [ "$CPU" != arm64 ]; then
			echo "ℹ️  the desktop app is built for Apple Silicon only: GUI steps skipped"
		elif build_gui_task darwin:package ARCH="$CPU"; then
			ditto -c -k --norsrc --noextattr --keepParent "$REPO_ROOT/bin/TinglyBox.app" "$ZIPS/tingly-box-gui-macos-$CPU.zip" \
				&& pass "built the macOS app bundle with task darwin:package" || fail "could not zip TinglyBox.app"
		fi ;;
	win32)
		if [ "$CPU" != amd64 ]; then
			echo "ℹ️  the desktop app is built for x64 Windows only: GUI steps skipped"
		elif build_gui_task windows:build PRODUCTION=true ARCH="$CPU"; then
			mkzip "$ZIPS/tingly-box-gui-windows-$CPU.zip" "$REPO_ROOT/bin/tingly-box-gui.exe" tingly-box-gui.exe \
				&& pass "built the Windows app with task windows:build" || fail "could not zip tingly-box-gui.exe"
		fi ;;
esac

# ---------------------------------------------------------------- 2. registry
echo "==> [2] local registry (Verdaccio)"
PORT=$((20000 + RANDOM % 20000))
REGISTRY="http://127.0.0.1:$PORT/"
npm_config_registry="$REAL_REGISTRY" npm install --prefix "$WORK/vd" --silent --no-audit --no-fund verdaccio \
	> "$LOG/verdaccio-install.log" 2>&1 || { tail -5 "$LOG/verdaccio-install.log"; die "could not install verdaccio"; }
cat > "$WORK/verdaccio.yaml" <<EOF
storage: $WORK/storage
auth:
  htpasswd:
    file: $WORK/htpasswd
    max_users: 10
packages:
  '**':
    access: \$all
    publish: \$authenticated
    unpublish: \$authenticated
max_body_size: 300mb
log: { type: file, path: $LOG/verdaccio.log, level: warn }
listen: 127.0.0.1:$PORT
EOF
"$WORK/vd/node_modules/.bin/verdaccio" --config "$WORK/verdaccio.yaml" > "$LOG/verdaccio.out" 2>&1 &
REG_PID=$!
for _ in $(seq 1 60); do curl -fs -m 2 "${REGISTRY}-/ping" >/dev/null 2>&1 && break; sleep 0.5; done
curl -fs -m 2 "${REGISTRY}-/ping" >/dev/null 2>&1 || die "registry did not start (see $LOG/verdaccio.out)"
TOKEN="$(curl -s -m 10 -X PUT -H 'content-type: application/json' \
	-d '{"name":"harness","password":"harness-pass-1","email":"h@example.invalid","type":"user"}' \
	"${REGISTRY}-/user/org.couchdb.user:harness" | "$PY" -I -c "import sys,json; print(json.load(sys.stdin).get('token',''))")"
[ -n "$TOKEN" ] || die "could not create a registry user"
printf 'registry=%s\n//127.0.0.1:%s/:_authToken=%s\n' "$REGISTRY" "$PORT" "$TOKEN" > "$WORK/npmrc"
export NPM_CONFIG_USERCONFIG="$WORK/npmrc" npm_config_cache="$WORK/npmcache"
pass "registry up on $REGISTRY (user harness)"

# ------------------------------------------------- 3. platform packages + shims
echo "==> [3] platform packages and shims, built like .github/workflows/npm.yml"
publish() { # <dir-or-tgz> <label>
	if (cd "$WORK" && npm publish "$1" --access public --tag latest) > "$LOG/publish-$(echo "$2" | tr '/@' '__').log" 2>&1; then
		return 0
	else
		echo "   publish of $2 failed:"; tail -4 "$LOG/publish-$(echo "$2" | tr '/@' '__').log"; return 1
	fi
}
for kind in cli gui; do
	flag=""; [ "$kind" = "gui" ] && flag="gui"
	"$SCRIPT_DIR/scripts/build-platform-packages.sh" "$VERSION" "$ZIPS" "$WORK/platform-$kind" $flag \
		> "$WORK/built-$kind.txt" 2> "$LOG/platform-$kind.err" || { cat "$LOG/platform-$kind.err"; die "platform packages ($kind) failed"; }
	n=0
	while read -r name; do
		name="${name%$'\r'}"
		[ -n "$name" ] || continue
		publish "$WORK/platform-$kind/$name" "$name" && n=$((n + 1)) || fail "publish $name"
	done < "$WORK/built-$kind.txt"
	[ "$n" -gt 0 ] && pass "published $n $kind platform package(s) at $VERSION: $(tr '\n' ' ' < "$WORK/built-$kind.txt")" || echo "ℹ️  no $kind platform package for this host"
done

# A scratch copy of build/npx: the real tree is never modified. node_modules is copied, not
# linked (esbuild resolves undici and unzipper from it; symlinks are unreliable under Git Bash).
NPX="$WORK/npx"; mkdir -p "$NPX"
cp -R "$SCRIPT_DIR/shared" "$SCRIPT_DIR/tingly-box" "$SCRIPT_DIR/tingly-box-gui" "$SCRIPT_DIR/package.json" "$NPX/"
rm -f "$NPX"/tingly-box/.bin.test-entry.js "$NPX"/tingly-box-gui/.bin.test-entry.js
[ -d "$SCRIPT_DIR/node_modules" ] || die "build/npx has no node_modules (run: cd build/npx && npm ci)"
cp -R "$SCRIPT_DIR/node_modules" "$NPX/node_modules"

build_shim() { # <pkg dir> <built list> <version>
	local pkg=$1 list=$2 ver=$3
	( cd "$NPX/$pkg" || exit 1
	  npm version "$ver" --no-git-tag-version >/dev/null || exit 1
	  while read -r name; do name="${name%$'\r'}"; [ -n "$name" ] && npm pkg set "optionalDependencies.${name}=${ver}"; done < "$list"
	  sed -i.bak "s|const BINARY_RELEASE_BRANCH = .*|const BINARY_RELEASE_BRANCH = 'v$ver';|g" bin.js && rm -f bin.js.bak
	  # the real registry for esbuild: the user config points npm at the local one
	  npm_config_registry="$REAL_REGISTRY" npx --yes esbuild@0.25.9 bin.js --bundle --platform=node --target=node18 \
		--format=esm --external:@aws-sdk/client-s3 \
		--banner:js="import{createRequire as __cr}from'module';const require=__cr(import.meta.url);" \
		--outfile=bin.bundled.js --log-level=warning || exit 1
	  mv bin.bundled.js bin.js
	  npm pkg delete dependencies
	  node --check bin.js || exit 1
	  mkdir -p "$WORK/tgz" && npm pack --pack-destination "$WORK/tgz" >/dev/null ) > "$LOG/shim-$pkg-$ver.log" 2>&1
}
for pkg in tingly-box tingly-box-gui; do
	list="$WORK/built-cli.txt"; [ "$pkg" = "tingly-box-gui" ] && list="$WORK/built-gui.txt"
	if build_shim "$pkg" "$list" "$VERSION"; then
		publish "$WORK/tgz/$pkg-$VERSION.tgz" "$pkg" && pass "shim $pkg bundled (single file, no deps) and published" || fail "publish $pkg"
	else
		fail "building shim $pkg failed (see $LOG/shim-$pkg-$VERSION.log)"; tail -5 "$LOG/shim-$pkg-$VERSION.log"; exit 1
	fi
done

# ---------------------------------------------------------------- 4. end to end
echo "==> [4] install from the registry and run, as a user would"
FRESH="$WORK/fresh"; mkdir -p "$FRESH"
CLI_PREFIX="$WORK/prefix-cli"; GUI_PREFIX="$WORK/prefix-gui"

# Where npm puts a global install: <prefix>/bin/<name> and <prefix>/lib/node_modules on Linux
# and macOS; <prefix>/<name> (an sh shim Git Bash can run) and <prefix>/node_modules on Windows.
npm_bin() { if [ "$HOST" = win32 ]; then printf '%s/%s' "$1" "$2"; else printf '%s/bin/%s' "$1" "$2"; fi; }
npm_mods() { if [ "$HOST" = win32 ]; then printf '%s/node_modules' "$1"; else printf '%s/lib/node_modules' "$1"; fi; }
# Where the shims keep their cache, per OS (shared/cachedir.js): XDG_CACHE_HOME, ~/Library/Caches
# (via HOME) and %LOCALAPPDATA%. CACHE_ENV is never empty (bash 3.2 and `set -u`).
set_cache_env() {
	case "$HOST" in
		linux) CACHE_ENV=("XDG_CACHE_HOME=$1") ;;
		darwin) CACHE_ENV=("HOME=$1") ;;
		win32) CACHE_ENV=("LOCALAPPDATA=$1") ;;
	esac
}

npm install -g --prefix "$CLI_PREFIX" "tingly-box@$VERSION" > "$LOG/install-cli.log" 2>&1 \
	&& pass "npm install -g tingly-box@$VERSION" || { fail "npm install -g tingly-box"; tail -5 "$LOG/install-cli.log"; }
ls "$(npm_mods "$CLI_PREFIX")/tingly-box/node_modules/@tingly-dev" 2>/dev/null | grep -q . \
	&& pass "the host's platform package was installed next to the shim" || fail "no nested platform package for $HOST-$CPU"

set_cache_env "$FRESH/cache-cli"
OUT="$(env "${CACHE_ENV[@]}" "$(npm_bin "$CLI_PREFIX" tingly-box)" version 2>&1)"
echo "$OUT" | grep -q "Installing binary from @tingly-dev/tingly-box-.*@$VERSION" && ! echo "$OUT" | grep -q "Downloading" \
	&& pass "first run installed the binary from the platform package, nothing downloaded" || { fail "first run did not use the platform package"; echo "$OUT" | head -5; }
echo "$OUT" | grep -Eq "Version:[[:space:]]+${TAG}[[:space:]]*\$" \
	&& pass "the binary reports $TAG (built from this tree)" || { fail "binary version is not $TAG"; echo "$OUT" | grep Version; }
env "${CACHE_ENV[@]}" "$(npm_bin "$CLI_PREFIX" tb)" --help >/dev/null 2>&1 \
	&& pass "tb (alias) runs from the cache" || fail "tb --help failed"
set_cache_env "$FRESH/cache-npx"
OUT="$(env "${CACHE_ENV[@]}" npm_config_cache="$WORK/npxcache" npm exec --yes --package "tingly-box@$VERSION" -- tingly-box version 2>&1)"
echo "$OUT" | grep -Eq "Version:[[:space:]]+${TAG}[[:space:]]*\$" \
	&& pass "npx style (npm exec) runs the same binary" || { fail "npm exec failed"; echo "$OUT" | tail -3; }

# A tb whose app was never published must say so, not fall back to another version. On Windows this
# is also the first place the shim's `npm.cmd` call runs for real.
if ls "$WORK"/platform-gui/@tingly-dev/tingly-box-gui-* >/dev/null 2>&1 || [ "$HOST" = linux ] || [ "$HOST" = win32 ] || [ "$HOST" = darwin ]; then
	PLANT="$WORK/plant/tingly-box"; mkdir -p "$PLANT"
	cp "$NPX/tingly-box/bin.js" "$PLANT/bin.js"
	node -e '
const fs = require("fs"), [src, dst] = process.argv.slice(1);
fs.writeFileSync(dst, JSON.stringify({ ...JSON.parse(fs.readFileSync(src, "utf8")), version: "999.999.999" }));
' "$NPX/tingly-box/package.json" "$PLANT/package.json"
	set_cache_env "$FRESH/cache-unpub"
	UNPUB_ENV=("HARNESS_UNPUB=1")
	if [ "$HOST" = linux ]; then
		# On Linux tb gui checks for a desktop session and GTK first; a fake ldconfig and DISPLAY get
		# past that on any host, so what is under test is the registry step.
		mkdir -p "$WORK/fakebin"
		printf '#!/bin/sh\nprintf "\\tlibgtk-3.so.0 (libc6,x86-64) => /x\\n\\tlibwebkit2gtk-4.1.so.0 (libc6,x86-64) => /x\\n"\n' > "$WORK/fakebin/ldconfig"
		chmod +x "$WORK/fakebin/ldconfig"
		UNPUB_ENV=("PATH=$WORK/fakebin:$PATH" "DISPLAY=:99")
	fi
	OUT="$(env "${UNPUB_ENV[@]}" "${CACHE_ENV[@]}" HOME="${HOME}" node "$PLANT/bin.js" gui 2>&1)"; RC=$?
	if [ "$RC" -ne 0 ] && echo "$OUT" | grep -q "not published on npm"; then
		pass "tb gui for an unpublished app version fails clearly (no fallback)"
	else
		fail "tb gui for an unpublished version should error with 'not published on npm' (exit $RC)"; echo "$OUT" | tail -4
	fi
fi

# ------------------------------------------------------------------ GUI launch
# Each launcher runs on a fresh home: it must return, the app must create ~/.tingly-box, keep
# serving 20 s later (it survives logging after the launcher is gone), and report the stamped
# version. The per-OS parts: how to get a display, which environment isolates the app, how to
# find and stop it.
kill_app() { # <marker>: stop the app of this run
	case "$HOST" in
		win32) taskkill //F //IM tingly-box-gui.exe //T >/dev/null 2>&1 || true ;;
		*) # by this run's own path, never `pkill -f <pattern>`: a pattern also found in this
		   # script's own command line would kill the script
		   for p in $(ps -axo pid,command | awk -v h="$1" 'index($0, h) && /tingly-box-gui/ && !/awk/ {print $1}'); do kill -9 "$p" 2>/dev/null; done ;;
	esac
}

launch_app() { # <name> <command...>
	local name=$1; shift
	local H="$FRESH/home-$name" CFG MARK RC XP="" BUSPID="" D
	rm -rf "$H"; mkdir -p "$H/xdg"; chmod 700 "$H/xdg"
	CFG="$H/.tingly-box"; MARK="$H"
	case "$HOST" in
		linux)
			D=$((50 + RANDOM % 40))
			Xvfb ":$D" -screen 0 1280x800x24 >/dev/null 2>&1 & XP=$!
			sleep 2
			eval "$(dbus-launch --sh-syntax 2>/dev/null)"; BUSPID="${DBUS_SESSION_BUS_PID:-}"
			( export HOME="$H" XDG_RUNTIME_DIR="$H/xdg" DISPLAY=":$D" XDG_CACHE_HOME="$H/cache"; "$@" > "$LOG/launch-$name.log" 2>&1 )
			RC=$? ;;
		darwin)
			# `open -a` hands the app to launchd: it runs with the real HOME, not this shell's. Start
			# from a clean real home (an ephemeral CI runner); never on a developer's machine.
			CFG="$HOME/.tingly-box"; MARK="$HOME/Library/Caches/tingly-box-gui"
			rm -rf "$CFG" "$MARK"
			( "$@" > "$LOG/launch-$name.log" 2>&1 )
			RC=$? ;;
		win32)
			( export USERPROFILE="$H" HOME="$H" LOCALAPPDATA="$H/AppData/Local" APPDATA="$H/AppData/Roaming"; "$@" > "$LOG/launch-$name.log" 2>&1 )
			RC=$? ;;
	esac
	[ "$RC" -eq 0 ] && pass "$name: the launcher returned (exit 0)" || { fail "$name: the launcher exited $RC"; tail -6 "$LOG/launch-$name.log"; }
	local code="" i
	# A cold start can be slow (Windows scans a freshly extracted exe before running it), so wait
	# up to 90 s for the first answer; then it must keep serving 20 s later.
	for i in $(seq 1 90); do
		code="$(curl -s -m 3 -o /dev/null -w '%{http_code}' http://127.0.0.1:12580/health 2>/dev/null || true)"
		[ -n "$code" ] && [ "$code" != "000" ] && break
		code=""; sleep 1
	done
	if [ -z "$code" ]; then
		fail "$name: the app never started serving within 90 s (it died, or never started)"
	else
		sleep 20
		code="$(curl -s -m 3 -o /dev/null -w '%{http_code}' http://127.0.0.1:12580/health 2>/dev/null || true)"
		[ -n "$code" ] && [ "$code" != "000" ] || { fail "$name: the app stopped serving within 20 s of starting"; code=""; }
	fi
	[ -n "$code" ] && pass "$name: the app still serves 20 s after launch (it survived logging past the launcher)"
	[ -f "$CFG/tingly-server.lock" ] && pass "$name: first launch created ~/.tingly-box" || fail "$name: no ~/.tingly-box after launch"
	[ "$(tr -d '\r' < "$CFG/tingly-server.version" 2>/dev/null)" = "$TAG" ] \
		&& pass "$name: the running app is the one built from this tree ($TAG)" \
		|| fail "$name: the running app reports '$(tr -d '\r' < "$CFG/tingly-server.version" 2>/dev/null)', not $TAG"
	# the app's own logs, for the artifact when something above failed
	[ -d "$CFG/log" ] && cp -R "$CFG/log" "$LOG/app-$name-log" 2>/dev/null
	if [ -z "$code" ]; then # show why in the job log too
		for f in "$CFG"/log/*; do [ -f "$f" ] && { echo "--- $f (tail)"; tail -15 "$f"; }; done 2>/dev/null
	fi
	kill_app "$MARK"
	[ -n "$XP" ] && kill "$XP" 2>/dev/null
	[ -n "$BUSPID" ] && kill "$BUSPID" 2>/dev/null
	sleep 1
	return 0
}

if ! ls "$WORK"/platform-gui/@tingly-dev/tingly-box-gui-* >/dev/null 2>&1; then
	echo "ℹ️  no GUI platform package for this host: GUI steps skipped"
else
	npm install -g --prefix "$GUI_PREFIX" "tingly-box-gui@$VERSION" > "$LOG/install-gui.log" 2>&1 \
		&& pass "npm install -g tingly-box-gui@$VERSION" || { fail "npm install -g tingly-box-gui"; tail -5 "$LOG/install-gui.log"; }
	CAN_LAUNCH=1
	[ "$GUI_LAUNCH" -eq 1 ] || CAN_LAUNCH=0
	if [ "$HOST" = linux ]; then command -v Xvfb >/dev/null 2>&1 && command -v dbus-launch >/dev/null 2>&1 || CAN_LAUNCH=0; fi
	if [ "$HOST" = darwin ] && [ "${CI:-}" != "true" ] && [ "${HARNESS_ALLOW_REAL_HOME:-}" != "1" ]; then
		echo "ℹ️  macOS starts the app with the real home directory: GUI launch only runs with CI=true (or HARNESS_ALLOW_REAL_HOME=1)"
		CAN_LAUNCH=0
	fi
	if [ "$CAN_LAUNCH" -eq 1 ]; then
		launch_app gui-direct "$(npm_bin "$GUI_PREFIX" tingly-box-gui)"
		launch_app tb-gui "$(npm_bin "$CLI_PREFIX" tb)" gui
		launch_app tb-app "$(npm_bin "$CLI_PREFIX" tb)" app
	else
		echo "ℹ️  GUI launch skipped"
	fi
fi

echo
echo "passed=$PASS failed=$FAIL  ($HOST-$CPU, version $TAG, registry $REGISTRY)"
if [ "$FAIL" -eq 0 ]; then echo "🎉 harness-npm: all checks passed"; else echo "💥 harness-npm: $FAIL check(s) FAILED (logs: $LOG; rerun with --keep)"; exit 1; fi
