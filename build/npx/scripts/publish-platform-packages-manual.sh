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
# Usage: publish-platform-packages-manual.sh <release-tag> [--dry-run] [--otp <code>]
#   release-tag  e.g. v0.260903.1 (the npm version is the tag without "v")
#   --dry-run    download + build + `npm publish --dry-run`, publish nothing
#   --otp CODE   authenticator code to pass to npm publish (TOTP accounts).
#                Without it npm prompts on the terminal for each package
#                (or opens the browser for a passkey/WebAuthn account).
#
# Env: GITHUB_REPO (default tingly-dev/tingly-box), WORK_DIR (default ./.platform-publish)
set -euo pipefail

TAG="${1:?usage: $0 <release-tag> [--dry-run] [--otp <code>]}"
shift
DRY_RUN=""
OTP=""
while [ $# -gt 0 ]; do
	case "$1" in
		--dry-run) DRY_RUN="--dry-run" ;;
		--otp) OTP="${2:?--otp needs a code}"; shift ;;
		--otp=*) OTP="${1#--otp=}" ;;
		*) echo "❌ unknown argument: $1" >&2; exit 1 ;;
	esac
	shift
done

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
# Read the list into an array first so npm keeps the terminal as stdin and
# can prompt for the one-time password (a `while read < file` loop would
# swallow the prompt and fail with EOTP).
mapfile -t NAMES < "$WORK_DIR/built.txt"
OTP_ARGS=()
if [ -n "$OTP" ]; then
	OTP_ARGS=(--otp "$OTP")
	echo "⚠️  a TOTP code is valid for ~30s; if a later package fails with EOTP, rerun with a fresh code (published ones are skipped)"
fi
for name in "${NAMES[@]}"; do
	if npm view "${name}@${VERSION}" version >/dev/null 2>&1; then
		echo "ℹ️  ${name}@${VERSION} already on npm, skipping"
		continue
	fi
	echo "📦 npm publish ${name}@${VERSION} ${DRY_RUN}"
	# The registry answers 409 "Failed to save packument" while it is still
	# processing a previous PUT for the same name (or a tombstone left by an
	# unpublish). Wait and retry a few times before giving up.
	attempt=1
	until (cd "$OUT_DIR/$name" && npm publish --access public ${DRY_RUN:+"$DRY_RUN"} "${OTP_ARGS[@]}") 2>&1 | tee "$WORK_DIR/publish.log"; test "${PIPESTATUS[0]}" -eq 0; do
		if [ "$attempt" -ge 4 ] || ! grep -q "409 Conflict" "$WORK_DIR/publish.log"; then
			echo "❌ publish of ${name}@${VERSION} failed" >&2
			exit 1
		fi
		echo "⏳ 409 from the registry, retrying in 60s (attempt $attempt/3)…"
		sleep 60
		attempt=$((attempt + 1))
	done
done

if [ -n "$DRY_RUN" ]; then
	echo "🧪 dry run finished, nothing published"
else
	echo "✅ done. Next: configure a Trusted Publisher on npmjs.com for each package"
	echo "   (repo $REPO, workflow npm.yml, environment production)."
fi
