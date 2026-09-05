#!/usr/bin/env bash
# Test the npx cli shim against a real GitHub release: build the published
# artifact like .github/workflows/npm.yml does (pin tag, esbuild single-file
# bundle), then verify the retired-dir sweep guards, an end-to-end
# download + `version` run, the failure output, and the platform-package
# install path. Details: .design/npm.md.
#
# Usage:   ./test-shim.sh <release-tag>
# Example: ./test-shim.sh v0.260819.0
set -euo pipefail

TAG="${1:?usage: $0 <release-tag>   e.g. $0 v0.260819.0}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PKG_DIR="$SCRIPT_DIR/tingly-box"
ESBUILD_VERSION="0.25.9"   # keep in sync with .github/workflows/npm.yml

WORK="$(mktemp -d)"
ENTRY="$PKG_DIR/.bin.test-entry.js"
trap 'rm -rf "$WORK" "$ENTRY"' EXIT

FAILED=0
pass() { echo "✅ $1"; }
fail() { echo "❌ $1"; FAILED=1; }

# --- build: same sequence as the CI publish job -----------------------------
# Deps are hoisted to build/npx/ (shared/ modules resolve them from there).
echo "==> [build] bundling shim like CI (tag $TAG)"
if [ ! -d "$SCRIPT_DIR/node_modules" ]; then
	(cd "$SCRIPT_DIR" && npm ci)
fi
sed "s|const BINARY_RELEASE_BRANCH = .*|const BINARY_RELEASE_BRANCH = '$TAG';|" \
	"$PKG_DIR/bin.js" > "$ENTRY"
npx --yes "esbuild@$ESBUILD_VERSION" "$ENTRY" --bundle --platform=node --target=node18 \
	--format=esm --external:@aws-sdk/client-s3 \
	--banner:js="import{createRequire as __cr}from'module';const require=__cr(import.meta.url);" \
	--outfile="$WORK/bin.js" --log-level=warning
node --check "$WORK/bin.js" && pass "build: bundle parses ($(du -h "$WORK/bin.js" | cut -f1))"

# The gui shim shares modules with the cli one — verify it also bundles to
# a parseable single file (no download/exec, just the build).
for pkg in tingly-box-gui; do
	npx --yes "esbuild@$ESBUILD_VERSION" "$SCRIPT_DIR/$pkg/bin.js" --bundle --platform=node --target=node18 \
		--format=esm --external:@aws-sdk/client-s3 \
		--banner:js="import{createRequire as __cr}from'module';const require=__cr(import.meta.url);" \
		--outfile="$WORK/$pkg.bin.js" --log-level=warning
	if node --check "$WORK/$pkg.bin.js"; then
		pass "build: $pkg bundle parses ($(du -h "$WORK/$pkg.bin.js" | cut -f1))"
	else
		fail "build: $pkg bundle does not parse"
	fi
done

# --- cold cache (macOS shim ignores XDG_CACHE_HOME; clear only the tag) -----
case "$(uname -s)" in
	Linux) export XDG_CACHE_HOME="$WORK/cache" ;;
	Darwin) rm -rf "$HOME/Library/Caches/tingly-box/$TAG" ;;
esac

# --- fake global install with planted dot-dirs ------------------------------
NM="$WORK/global/node_modules"
RETIRED="$NM/.tingly-box-BweFGdMf"        # npm retire shape: .<name>-<8 alnum>
SIBLING="$NM/.tingly-box-gui-Ab12Cd34"    # another package's retire dir
HUMAN="$NM/.tingly-box-backup"            # human-made dir, must never be touched
mkdir -p "$NM/tingly-box" "$RETIRED/junk" "$SIBLING" "$HUMAN"
cp "$WORK/bin.js" "$NM/tingly-box/bin.js"
# The shim reads its own version from package.json (platform-package match).
VERSION="${TAG#v}"
(cd "$PKG_DIR" && npm version "$VERSION" --no-git-tag-version --allow-same-version >/dev/null)
cp "$PKG_DIR/package.json" "$NM/tingly-box/package.json"
git -C "$PKG_DIR" checkout -q package.json 2>/dev/null || true

run_shim() {
	node "$NM/tingly-box/bin.js" --transport-version="$TAG" version \
		> "$WORK/run.log" 2>&1
}

# --- T1: fresh parent mtime -> sweep must skip ------------------------------
echo "==> [T1] fresh node_modules mtime (npm transaction may be in flight)"
if run_shim; then pass "T1: shim ran (exit 0)"; else fail "T1: shim failed:"; tail -5 "$WORK/run.log"; fi
[ -d "$RETIRED" ] && pass "T1: retire dir untouched while mtime fresh" \
	|| fail "T1: retire dir was deleted despite fresh mtime (rollback race!)"

# --- T2: old parent mtime -> exactly the retire dir swept -------------------
echo "==> [T2] old node_modules mtime (real leftover scenario)"
touch -t 202001010000 "$NM"   # BSD/GNU portable "long ago"
if run_shim; then pass "T2: shim ran (exit 0)"; else fail "T2: shim failed:"; tail -5 "$WORK/run.log"; fi
[ ! -d "$RETIRED" ] && pass "T2: own retire leftover swept" \
	|| fail "T2: own retire leftover NOT swept"
[ -d "$SIBLING" ] && pass "T2: sibling package's retire dir untouched" \
	|| fail "T2: sibling package's retire dir was deleted"
[ -d "$HUMAN" ] && pass "T2: human-made dot-dir untouched" \
	|| fail "T2: human-made dot-dir was deleted"

# --- T3: the binary really ran as <tag> -------------------------------------
echo "==> [T3] end-to-end: downloaded binary reports the requested version"
grep -Eq "Version:[[:space:]]+$TAG" "$WORK/run.log" \
	&& pass "T3: binary executed and reported $TAG" \
	|| { fail "T3: version output missing:"; tail -10 "$WORK/run.log"; }

# --- T4: stale version-cache sweep (Linux only: cache root is sandboxed) ----
if [ "$(uname -s)" = "Linux" ]; then
	echo "==> [T4] stale ~/.cache/tingly-box/<tag> dirs swept, guards respected"
	CACHE_ROOT="$XDG_CACHE_HOME/tingly-box"
	OLD1="$CACHE_ROOT/v0.0.1"       # oldest stale tag -> must be swept
	OLD2="$CACHE_ROOT/v0.0.2"       # newest stale tag -> kept as rollback
	FRESH="$CACHE_ROOT/v9.9.9"      # fresh mtime -> may be mid-download, kept
	HUMANC="$CACHE_ROOT/backup"     # non-tag dir -> never touched
	PTR="$CACHE_ROOT/current"       # file (future version pointer) -> kept
	mkdir -p "$OLD1/bin" "$OLD2/bin" "$FRESH/bin" "$HUMANC"
	echo x > "$PTR"
	touch -t 202001010000 "$OLD1"
	touch -t 202101010000 "$OLD2"
	if run_shim; then pass "T4: shim ran (exit 0)"; else fail "T4: shim failed:"; tail -5 "$WORK/run.log"; fi
	[ ! -d "$OLD1" ] && pass "T4: oldest stale tag dir swept" \
		|| fail "T4: oldest stale tag dir NOT swept"
	[ -d "$OLD2" ] && pass "T4: newest stale tag dir kept as rollback" \
		|| fail "T4: rollback tag dir was deleted"
	[ -d "$FRESH" ] && pass "T4: fresh tag dir untouched (concurrency guard)" \
		|| fail "T4: fresh tag dir was deleted despite fresh mtime"
	[ -d "$HUMANC" ] && pass "T4: non-tag dir untouched" \
		|| fail "T4: non-tag dir was deleted"
	[ -f "$PTR" ] && pass "T4: pointer file untouched" \
		|| fail "T4: pointer file was deleted"
	[ -d "$CACHE_ROOT/$TAG" ] && pass "T4: current tag dir kept" \
		|| fail "T4: current tag dir was deleted"
else
	echo "==> [T4] skipped (non-Linux: cache root not sandboxed by XDG_CACHE_HOME)"
fi

# --- T5: download failure is reported without a stack trace ----------------
echo "==> [T5] non-existent tag: failure output is structured, exit non-zero"
if node "$NM/tingly-box/bin.js" --transport-version=v0.0.0-nonexistent version \
	> "$WORK/fail.log" 2>&1; then
	fail "T5: shim exited 0 on a missing release"
else
	pass "T5: shim exited non-zero on a missing release"
fi
grep -q "Download failed: 404" "$WORK/fail.log" && grep -q "Next steps" "$WORK/fail.log" \
	&& pass "T5: failure reason and next steps printed" \
	|| { fail "T5: structured failure output missing:"; tail -10 "$WORK/fail.log"; }
grep -q "Stack:" "$WORK/fail.log" \
	&& fail "T5: stack trace leaked into the failure output" \
	|| pass "T5: no stack trace in the failure output"

# --- T6: platform package (the npm install path) is used, no download ------
# Build tingly-box-linux-x64 from the release zip like the publish workflow,
# plant it where npm nests optional deps of a global install, and check the
# shim installs from it into the versioned cache without touching GitHub.
if [ "$(uname -s)" = "Linux" ] && [ "$(uname -m)" = "x86_64" ]; then
	echo "==> [T6] platform package present: binary installed from it, no download"
	mkdir -p "$WORK/zips"
	curl -sSfL -o "$WORK/zips/tingly-box-linux-amd64.zip" \
		"https://github.com/tingly-dev/tingly-box/releases/download/$TAG/tingly-box-linux-amd64.zip"
	"$SCRIPT_DIR/scripts/build-platform-packages.sh" "$VERSION" "$WORK/zips" "$WORK/platform" >/dev/null 2>&1 \
		&& pass "T6: platform package built from the release zip" \
		|| fail "T6: build-platform-packages.sh failed"
	mkdir -p "$NM/tingly-box/node_modules"
	cp -r "$WORK/platform/tingly-box-linux-x64" "$NM/tingly-box/node_modules/"
	rm -rf "$XDG_CACHE_HOME/tingly-box/$TAG"
	if node "$NM/tingly-box/bin.js" version > "$WORK/pkg.log" 2>&1; then
		pass "T6: shim ran (exit 0)"
	else
		fail "T6: shim failed:"; tail -5 "$WORK/pkg.log"
	fi
	grep -q "Installing binary from tingly-box-linux-x64@$VERSION" "$WORK/pkg.log" \
		&& ! grep -q "Downloading" "$WORK/pkg.log" \
		&& pass "T6: binary came from the platform package, nothing downloaded" \
		|| { fail "T6: expected the platform package path:"; head -5 "$WORK/pkg.log"; }
	grep -Eq "Version:[[:space:]]+$TAG" "$WORK/pkg.log" \
		&& pass "T6: binary executed and reported $TAG" \
		|| fail "T6: version output missing"
	[ -x "$XDG_CACHE_HOME/tingly-box/$TAG/bin/tingly-box" ] \
		&& pass "T6: binary copied into the versioned cache dir" \
		|| fail "T6: cache copy missing"
	# Version mismatch (partial upgrade) must fall back to the release download.
	sed -i 's/"version": "[^"]*"/"version": "0.0.1"/' "$NM/tingly-box/node_modules/tingly-box-linux-x64/package.json"
	node "$NM/tingly-box/bin.js" version > "$WORK/mismatch.log" 2>&1 || true
	grep -q "does not match tingly-box@$VERSION" "$WORK/mismatch.log" \
		&& ! grep -q "Installing binary from" "$WORK/mismatch.log" \
		&& pass "T6: mismatched platform package ignored, download path taken" \
		|| { fail "T6: mismatch fallback missing:"; head -5 "$WORK/mismatch.log"; }
	rm -rf "$NM/tingly-box/node_modules"
else
	echo "==> [T6] skipped (needs linux/x86_64 to run the linux-x64 package)"
fi

echo
if [ "$FAILED" -eq 0 ]; then
	echo "🎉 All shim tests passed for $TAG"
else
	echo "💥 Some shim tests FAILED for $TAG"
	exit 1
fi
