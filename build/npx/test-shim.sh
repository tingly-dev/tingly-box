#!/usr/bin/env bash
# Test the npx cli shim against a real GitHub release: build the published
# artifact like .github/workflows/npm.yml does (pin tag, esbuild single-file
# bundle), then verify the retired-dir sweep guards and an end-to-end
# download + `version` run. Details: .design/npm.md.
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

# The other two shims share modules with the cli one — verify they also
# bundle to a parseable single file (no download/exec, just the build).
for pkg in tingly-box-gui tingly-box-bundle; do
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

# --- T5: download failure prints the bundle switch (installed-bin mode) ----
echo "==> [T5] non-existent tag: failure output names the uninstall-first switch"
if node "$NM/tingly-box/bin.js" --transport-version=v0.0.0-nonexistent version \
	> "$WORK/fail.log" 2>&1; then
	fail "T5: shim exited 0 on a missing release"
else
	pass "T5: shim exited non-zero on a missing release"
fi
grep -q "npm uninstall -g tingly-box" "$WORK/fail.log" \
	&& grep -q "npm install -g tingly-box-bundle@0.0.0-nonexistent" "$WORK/fail.log" \
	&& pass "T5: uninstall-first switch to tingly-box-bundle printed" \
	|| { fail "T5: switch hint missing:"; tail -10 "$WORK/fail.log"; }
grep -q "Stack:" "$WORK/fail.log" \
	&& fail "T5: stack trace leaked into the failure output" \
	|| pass "T5: no stack trace in the failure output"

echo
if [ "$FAILED" -eq 0 ]; then
	echo "🎉 All shim tests passed for $TAG"
else
	echo "💥 Some shim tests FAILED for $TAG"
	exit 1
fi
