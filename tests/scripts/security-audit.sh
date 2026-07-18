#!/bin/sh
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$repo_dir"

secret_pattern='(sk_live_|rk_live_|whsec_)[A-Za-z0-9]{24,}'
if rg --hidden --glob '!**/node_modules/**' --glob '!**/.next/**' --glob '!**/.git/**' "$secret_pattern" apps services packages infra .github 2>/dev/null; then
  echo "potential committed production secret detected" >&2
  exit 1
fi

go test ./...
pnpm lint
pnpm typecheck
pnpm test

if command -v govulncheck >/dev/null 2>&1; then
  govulncheck ./...
else
  echo "note: govulncheck not installed; the security workflow runs it in CI"
fi

if [ "${PARITYLAB_OFFLINE:-0}" != "1" ]; then
  pnpm audit --prod --audit-level high
fi

echo "local security audit passed"
