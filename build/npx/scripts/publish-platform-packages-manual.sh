#!/usr/bin/env bash
# One-off manual publish of the per-platform binary packages
# (tingly-box-linux-x64, …) from a maintainer's machine.
#
# Needed once per package that does not exist on npm yet: a Trusted Publisher
# (OIDC) can only be configured on an existing package, so the first version
# has to be published with an interactive 2FA login. After that
# .github/workflows/npm.yml publishes every version via OIDC.
#
# Downloads the release zips straight from GitHub Releases with curl (no gh),
# builds the packages with build-platform-packages.sh and runs `npm publish`
# for each one; npm prompts for 2FA (browser / passkey) on every publish.
#
# Usage: publish-platform-packages-manual.sh <release-tag> [--dry-run]
#   release-tag  e.g. v0.260903.1 (the npm version is the tag without "v")
#   --dry-run    download + build + `npm publish --dry-run`, publish nothing
#
# Env: GITHUB_REPO (default tingly-dev/tingly-box), WORK_DIR (default ./.platform-publish)
set -euo pipefail

TAG="${1:?usage: $0 <release-tag> [--dry-run]}"
DRY_RUN=""
[ "${2:-}" = "--dry-run" ] && DRY_RUN="--dry-run"

if [[ ! "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9]+)?$ ]]; then
	echo "❌ invalid tag '$TAG', expected e.g. v1.0.0 or v1.0.0-beta" >&2
	exit 1
fi
VERSION="${TAG#v}"
REPO="${GITHUB_REPO:-tingly-dev/tingly-box}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NPX_DIR="$(dirname "$SCRIPT_DIR")"
WORK_DIR="${WORK_DIR:-$PWD/.platform-publish}"
ZIP_DIR="$WORK_DIR/zips"
OUT_DIR="$WORK_DIR/platform"

for tool in curl unzip node npm; do
	command -v "$tool" >/dev/null || { echo "❌ $tool is required" >&2; exit 1; }
done

if [ -z "$DRY_RUN" ]; then
	if ! npm whoami >/dev/null 2>&1; then
		echo "❌ not logged in to npm. Run 'npm login' first (passkey/WebAuthn 2FA)." >&2
		exit 1
	fi
	echo "👤 publishing as $(npm whoami) to $(npm config get registry)"
fi

# ---- download release zips (curl, follows the GitHub CDN redirect) ---------
mkdir -p "$ZIP_DIR"
ZIPS="$(node -e '
import("'"$NPX_DIR"'/shared/platform.js").then(m => {
  for (const { zip } of Object.values(m.PLATFORM_PACKAGES)) console.log(zip);
});')"
while read -r zip; do
	url="https://github.com/$REPO/releases/download/$TAG/$zip"
	if [ -s "$ZIP_DIR/$zip" ]; then
		echo "ℹ️  $zip already downloaded"
		continue
	fi
	echo "📥 $url"
	curl -fL --retry 3 --retry-delay 2 -o "$ZIP_DIR/$zip.part" "$url"
	mv "$ZIP_DIR/$zip.part" "$ZIP_DIR/$zip"
done <<< "$ZIPS"
ls -lh "$ZIP_DIR"

# ---- build ---------------------------------------------------------------
"$SCRIPT_DIR/build-platform-packages.sh" "$VERSION" "$ZIP_DIR" "$OUT_DIR" > "$WORK_DIR/built.txt"
EXPECTED="$(printf '%s\n' "$ZIPS" | wc -l | tr -d ' ')"
BUILT="$(wc -l < "$WORK_DIR/built.txt" | tr -d ' ')"
if [ "$BUILT" -ne "$EXPECTED" ]; then
	echo "❌ built $BUILT packages, expected $EXPECTED (a release zip is missing)" >&2
	exit 1
fi
echo "✅ built $BUILT packages:"
cat "$WORK_DIR/built.txt"

# ---- publish -------------------------------------------------------------
# No --provenance: that needs the CI OIDC token and fails on a laptop.
while read -r name; do
	if npm view "${name}@${VERSION}" version >/dev/null 2>&1; then
		echo "ℹ️  ${name}@${VERSION} already on npm, skipping"
		continue
	fi
	echo "📦 npm publish ${name}@${VERSION} ${DRY_RUN}"
	(cd "$OUT_DIR/$name" && npm publish --access public $DRY_RUN)
done < "$WORK_DIR/built.txt"

if [ -n "$DRY_RUN" ]; then
	echo "🧪 dry run finished, nothing published"
else
	echo "✅ done. Next: configure a Trusted Publisher on npmjs.com for each package"
	echo "   (repo $REPO, workflow npm.yml, environment production)."
fi
