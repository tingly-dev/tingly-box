#!/usr/bin/env bash
# Usage: npm-publish.sh <version> <dist-tag> <dir>...
#
# Publish each package directory (with provenance, public) under <dist-tag>,
# concurrently. Idempotent: a version already on the registry is a skip, not a
# failure. `npm view` can lag behind a publish by minutes on the read replicas,
# so the registry's own "previously published" rejection counts as "already
# there" too. Each package logs to its own file, printed only when it fails.
set -euo pipefail

[ "$#" -ge 2 ] || { echo "usage: $0 <version> <dist-tag> <dir>..." >&2; exit 2; }
VERSION="$1" NPM_TAG="$2"
shift 2
[ "$#" -gt 0 ] || { echo "Nothing to publish."; exit 0; }

LOGS="$(mktemp -d)"
publish_one() {
  local dir="$1" log="$2" name
  name="$(cd "$dir" && node -p 'require("./package.json").name')"
  echo "$name" > "$log.name"
  if npm view "${name}@${VERSION}" version >/dev/null 2>&1; then
    echo skip > "$log.status"; return 0
  fi
  if (cd "$dir" && npm publish --provenance --access public --tag "$NPM_TAG") > "$log" 2>&1; then
    echo ok > "$log.status"
  elif grep -qiE "previously published|cannot publish over|EPUBLISHCONFLICT|already exists" "$log"; then
    echo skip > "$log.status"
  else
    echo fail > "$log.status"
  fi
}

i=0
for dir in "$@"; do
  i=$((i + 1))
  echo "📦 Publishing $dir @ ${VERSION} (tag: ${NPM_TAG})..."
  publish_one "$dir" "$LOGS/$i" &
done
wait

FAILED=0
i=0
for dir in "$@"; do
  i=$((i + 1))
  log="$LOGS/$i"
  name="$(cat "$log.name" 2>/dev/null || echo "$dir")"
  case "$(cat "$log.status" 2>/dev/null)" in
    ok)   echo "✅ ${name}@${VERSION} published" ;;
    skip) echo "ℹ️  ${name}@${VERSION} already on npm, skipped" ;;
    *)    echo "❌ ${name}@${VERSION} failed:"; sed 's/^/    /' "$log" 2>/dev/null || true; FAILED=1 ;;
  esac
done
[ "$FAILED" -eq 0 ]
