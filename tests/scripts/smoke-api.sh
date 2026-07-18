#!/bin/sh
set -eu

api_url=${PARITYLAB_API_URL:-http://127.0.0.1:8080}
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT INT TERM

curl --fail --silent --show-error "$api_url/healthz" >"$tmp_dir/health.json"
jq -e '.status == "ok" and .mode == "sandbox"' "$tmp_dir/health.json" >/dev/null

curl --fail --silent --show-error "$api_url/v1/scenarios" >"$tmp_dir/scenarios.json"
jq -e '.object == "list" and (.data | length) > 0' "$tmp_dir/scenarios.json" >/dev/null

idempotency_key="smoke-$(date +%s)-$$"
curl --fail --silent --show-error \
  -H 'Content-Type: application/json' \
  -H "Idempotency-Key: $idempotency_key" \
  -d '{"scenario_id":"checkout-duplicate","fault":"duplicate"}' \
  "$api_url/v1/runs" >"$tmp_dir/run.json"

run_id=$(jq -er '.id' "$tmp_dir/run.json")
jq -e '.status == "passed" and .environment == "sandbox"' "$tmp_dir/run.json" >/dev/null
curl --fail --silent --show-error "$api_url/v1/runs/$run_id/report" >"$tmp_dir/report.json"
jq -e --arg id "$run_id" '.run.id == $id and (.assertions | length) > 0' "$tmp_dir/report.json" >/dev/null

echo "API smoke passed for $run_id"
