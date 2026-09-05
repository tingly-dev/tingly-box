#!/usr/bin/env bash
# Build the per-platform binary packages (tingly-box-linux-x64, …) that the
# `tingly-box` package pulls in as optionalDependencies. Each package is the
# raw Go binary from the release zip plus a package.json pinned to the same
# version as the shim. Used by .github/workflows/npm.yml and test-shim.sh.
# Design: .design/npm.md ("F. Platform packages").
#
# Usage: build-platform-packages.sh <version> <zip-dir> <out-dir>
#   version  npm version to stamp (release tag without the leading "v")
#   zip-dir  directory holding the release zips (tingly-box-<os>-<arch>.zip);
#            platforms whose zip is absent are skipped with a notice
#   out-dir  one package directory per platform is written under here
# Prints the names of the packages built, one per line, on stdout.
set -euo pipefail

VERSION="${1:?usage: $0 <version> <zip-dir> <out-dir>}"
ZIP_DIR="$(cd "${2:?usage: $0 <version> <zip-dir> <out-dir>}" && pwd)"
OUT_DIR="${3:?usage: $0 <version> <zip-dir> <out-dir>}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NPX_DIR="$(dirname "$SCRIPT_DIR")"

mkdir -p "$OUT_DIR"
OUT_DIR="$(cd "$OUT_DIR" && pwd)"

# key|name|zip lines from the single source of truth in shared/platform.js.
PLATFORMS="$(node -e '
import("'"$NPX_DIR"'/shared/platform.js").then(m => {
  for (const [key, { name, zip }] of Object.entries(m.PLATFORM_PACKAGES)) console.log(`${key}|${name}|${zip}`);
});')"

while IFS='|' read -r key name zip; do
	os="${key%-*}"
	cpu="${key##*-}"
	zip_path="$ZIP_DIR/$zip"
	if [ ! -f "$zip_path" ]; then
		echo "skip $name: $zip not found in $ZIP_DIR" >&2
		continue
	fi
	binary="tingly-box"
	[ "$os" = "win32" ] && binary="tingly-box.exe"

	pkg_dir="$OUT_DIR/$name"
	rm -rf "$pkg_dir"
	mkdir -p "$pkg_dir/bin"
	# The release zip holds the binary at its top level; extract just that.
	unzip -q -o -j "$zip_path" "$binary" -d "$pkg_dir/bin"
	[ "$os" = "win32" ] || chmod 755 "$pkg_dir/bin/$binary"

	cat > "$pkg_dir/package.json" <<JSON
{
  "name": "$name",
  "version": "$VERSION",
  "description": "tingly-box binary for $os-$cpu. Installed automatically as an optional dependency of the tingly-box package; not meant to be installed directly.",
  "homepage": "https://github.com/tingly-dev/tingly-box",
  "repository": {
    "type": "git",
    "url": "https://github.com/tingly-dev/tingly-box.git"
  },
  "license": "MPL-2.0",
  "author": "Tingly Dev",
  "publishConfig": {
    "access": "public"
  },
  "os": [
    "$os"
  ],
  "cpu": [
    "$cpu"
  ],
  "files": [
    "bin/"
  ]
}
JSON
	cat > "$pkg_dir/README.md" <<MD
# $name

The tingly-box binary for $os-$cpu. This package is pulled in automatically
by the [\`tingly-box\`](https://www.npmjs.com/package/tingly-box) package as
an optional dependency; install that one instead:

\`\`\`bash
npm install -g tingly-box
\`\`\`
MD
	echo "built $name@$VERSION ($(du -h "$pkg_dir/bin/$binary" | cut -f1))" >&2
	echo "$name"
done <<< "$PLATFORMS"
