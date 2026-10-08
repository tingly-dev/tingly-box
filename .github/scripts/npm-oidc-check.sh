#!/usr/bin/env bash
# Usage: npm-oidc-check.sh <package>...
#
# Do npm's Trusted Publishing (OIDC) token exchange for each package up front.
# `npm publish` silently falls back to token auth when the exchange is rejected
# and then fails with a bare E404; this makes the registry's reason visible and
# fails the job before anything is published. Needs `id-token: write`.
set -euo pipefail

[ "$#" -gt 0 ] || { echo "usage: $0 <package>..." >&2; exit 2; }

ID_TOKEN=$(curl -sSf -H "Authorization: Bearer $ACTIONS_ID_TOKEN_REQUEST_TOKEN" \
  "${ACTIONS_ID_TOKEN_REQUEST_URL}&audience=npm:registry.npmjs.org" | jq -r .value)
echo "id-token claims npm matches against:"
node -e 'const c=JSON.parse(Buffer.from(process.argv[1].split(".")[1],"base64url")); console.log(JSON.stringify({repository:c.repository,repository_owner:c.repository_owner,workflow_ref:c.workflow_ref,job_workflow_ref:c.job_workflow_ref,environment:c.environment,event_name:c.event_name,ref:c.ref},null,2))' "$ID_TOKEN"

FAIL=0
for pkg in "$@"; do
  escaped=$(printf '%s' "$pkg" | sed 's|/|%2F|')
  body=$(curl -sS -o /dev/stdout -w '\n%{http_code}' -X POST \
    -H "Authorization: Bearer $ID_TOKEN" \
    "https://registry.npmjs.org/-/npm/v1/oidc/token/exchange/package/$escaped")
  code=${body##*$'\n'}
  resp=${body%$'\n'*}
  # The registry answers 201 Created (not 200) when it mints a token; accept
  # any 2xx. Never echo the body on success: it carries the token.
  case "$code" in
    2??) echo "✅ $pkg: exchange OK (HTTP $code)" ;;
    *)
      echo "❌ $pkg: exchange HTTP $code: $(printf '%s' "$resp" | jq -r '.message // .error // "(no message)"' 2>/dev/null || echo "(unparseable response)")"
      FAIL=1
      ;;
  esac
done

[ "$FAIL" -eq 0 ] || {
  echo "The registry has no Trusted Publisher on that package that matches this run AND allows 'npm publish'."
  echo "Check on npmjs.com (package Settings -> Trusted Publisher): org tingly-dev, repo tingly-box, workflow npm.yml,"
  echo "environment production, and Allowed actions includes 'npm publish' (configs created after 2026-09-03 default"
  echo "to stage publish only). Or: npm trust list <pkg> / npm trust github <pkg> --repo tingly-dev/tingly-box --file npm.yml --env production --allow-publish"
  exit 1
}
