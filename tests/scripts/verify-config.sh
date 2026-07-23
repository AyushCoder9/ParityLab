#!/bin/sh
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$repo_dir"

for file in infra/grafana/dashboards/*.json; do
  jq -e . "$file" >/dev/null
done

docker compose -f infra/compose.yaml config --quiet
docker compose -f infra/compose.test.yaml config --quiet
docker compose -f infra/compose.auth-test.yaml config --quiet
docker compose -f infra/compose.webhook-test.yaml config --quiet
tests/scripts/verify-openapi-contract.sh

if command -v actionlint >/dev/null 2>&1; then
  actionlint .github/workflows/*.yml
else
  echo "note: actionlint is not installed; CI validates workflow execution"
fi

echo "configuration validation passed"
