#!/bin/sh
set -eu

if [ "${PARITYLAB_CONFIRM_FRESH:-0}" != "1" ]; then
  echo "Refusing to recreate the dedicated test database. Re-run with PARITYLAB_CONFIRM_FRESH=1." >&2
  exit 2
fi

for command in curl docker jq openssl; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "required command is missing: $command" >&2
    exit 2
  fi
done

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
compose_file="$repo_dir/infra/compose.test.yaml"
project_name=paritylab-persistence-contract
api_port=${PARITYLAB_PERSISTENCE_API_PORT:-18081}
api_origin="http://127.0.0.1:${api_port}"
web_origin="http://127.0.0.1:3203"
temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/paritylab-persistence.XXXXXX")
cookie_jar="$temp_dir/session.cookies"

cleanup() {
  status=$?
  trap - EXIT INT TERM
  if [ "$status" -ne 0 ]; then
    docker compose -p "$project_name" -f "$compose_file" logs api-test stripe-mock postgres-test >&2 || true
  fi
  docker compose -p "$project_name" -f "$compose_file" down --volumes --remove-orphans >/dev/null 2>&1 || true
  rm -rf "$temp_dir"
  exit "$status"
}
trap cleanup EXIT INT TERM

await_api() {
  attempt=0
  until curl --fail --silent "$api_origin/healthz" >/dev/null 2>&1; do
    attempt=$((attempt + 1))
    if [ "$attempt" -ge 90 ]; then
      docker compose -p "$project_name" -f "$compose_file" logs api-test >&2
      echo "API did not become ready" >&2
      exit 1
    fi
    sleep 1
  done
}

register() {
  status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/register.json" -c "$cookie_jar" \
    -H 'Content-Type: application/json' -H "Origin: $web_origin" \
    --data '{"email":"persistence-contract@example.test","password":"QA_LOG_SENTINEL_NeverLog_42!","workspace_name":"persistence workspace","project_name":"persistence project"}' \
    "$api_origin/v1/auth/register")
  if [ "$status" != "201" ]; then
    cat "$temp_dir/register.json" >&2
    echo "registration returned HTTP $status" >&2
    return 1
  fi
}

deliver_webhook() {
  output_file=$1
  timestamp=$(date +%s)
  body='{"id":"evt_persistence_restart","object":"event","type":"payment_intent.succeeded","livemode":false,"data":{"object":{"id":"pi_persistence_restart"}}}'
  signature=$(printf '%s.%s' "$timestamp" "$body" | openssl dgst -sha256 -hmac whsec_paritylab_demo -hex | awk '{print $NF}')
  curl --fail --silent --show-error \
    -H 'Content-Type: application/json' \
    -H "Stripe-Signature: t=${timestamp},v1=${signature}" \
    --data "$body" \
    "$api_origin/hooks/stripe/demo" >"$output_file"
}

validate_stripe_connection() {
  status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/stripe-connection.json" \
    -b "$cookie_jar" -H 'Content-Type: application/json' -H "Origin: $web_origin" \
    --data '{"secret_key":"sk_test_paritylab_contract","sandbox_name":"QA Sandbox"}' \
    "$api_origin/v1/connections/stripe/validate")
  if [ "$status" != "201" ]; then
    cat "$temp_dir/stripe-connection.json" >&2
    echo "Stripe connection validation returned HTTP $status" >&2
    return 1
  fi
  jq -e '
    .stripe_account_id == "acct_mock_sandbox" and
    .sandbox_name == "QA Sandbox" and
    .status == "connected" and
    (.id | type == "string") and
    (.secret_key == null) and
    (.ciphertext == null)
  ' "$temp_dir/stripe-connection.json" >/dev/null
  jq -er '.id' "$temp_dir/stripe-connection.json"
}

create_stripe_run() {
  connection_id=$1
  idempotency_key=$2
  headers_file=$3
  output_file=$4
  status=$(curl --silent --show-error --write-out '%{http_code}' --output "$output_file" -D "$headers_file" \
    -b "$cookie_jar" -H 'Content-Type: application/json' -H "Origin: $web_origin" \
    -H "Idempotency-Key: ${idempotency_key}" \
    --data "{\"connection_id\":\"${connection_id}\",\"amount_minor\":1099,\"currency\":\"usd\"}" \
    "$api_origin/v1/stripe/payment-intents")
  if [ "$status" != "201" ]; then
    cat "$output_file" >&2
    echo "Stripe PaymentIntent run returned HTTP $status" >&2
    return 1
  fi
}

cd "$repo_dir"
docker compose -p "$project_name" -f "$compose_file" down --volumes --remove-orphans >/dev/null 2>&1 || true
PARITYLAB_PERSISTENCE_API_PORT="$api_port" docker compose -p "$project_name" -f "$compose_file" up -d --wait
await_api
register

live_key_status=$(curl --silent --output "$temp_dir/stripe-live-key-error.json" --write-out '%{http_code}' \
  -b "$cookie_jar" -H 'Content-Type: application/json' -H "Origin: $web_origin" \
  --data '{"secret_key":"sk_live_must_never_be_accepted","sandbox_name":"Unsafe"}' \
  "$api_origin/v1/connections/stripe/validate")
[ "$live_key_status" = "400" ]
jq -e '.error.code == "sandbox_key_required" and (.error.message | test("live"; "i"))' "$temp_dir/stripe-live-key-error.json" >/dev/null

connection_id=$(validate_stripe_connection)
invalid_currency_status=$(curl --silent --output "$temp_dir/stripe-invalid-currency.json" --write-out '%{http_code}' \
  -b "$cookie_jar" -H 'Content-Type: application/json' -H "Origin: $web_origin" \
  -H 'Idempotency-Key: stripe-invalid-currency-contract' \
  --data "{\"connection_id\":\"${connection_id}\",\"amount_minor\":1099,\"currency\":\"USD\"}" \
  "$api_origin/v1/stripe/payment-intents")
[ "$invalid_currency_status" = "400" ]
jq -e '.error.code == "invalid_currency"' "$temp_dir/stripe-invalid-currency.json" >/dev/null

create_stripe_run "$connection_id" stripe-payment-restart-contract "$temp_dir/stripe-run-first.headers" "$temp_dir/stripe-run-first.json"
stripe_run_id=$(jq -er 'select(.environment == "sandbox") | select(.stripe_object_id | startswith("pi_mock_")) | .id' "$temp_dir/stripe-run-first.json")
curl --fail --silent --show-error -b "$cookie_jar" "$api_origin/v1/runs/$stripe_run_id/events" >"$temp_dir/stripe-events-first.json"
jq -e '
  [.data[].evidence.paritylab_correlation_id] as $ids |
  ($ids | length) > 0 and ($ids | unique | length) == 1 and
  ($ids[0] | test("^plcorr_[a-f0-9]{24}$"))
' "$temp_dir/stripe-events-first.json" >/dev/null
curl --fail --silent --show-error -b "$cookie_jar" "$api_origin/v1/runs/$stripe_run_id/report" >"$temp_dir/stripe-report-first.json"
jq -e --arg run_id "$stripe_run_id" '.run.id == $run_id and .state.balanced == true' "$temp_dir/stripe-report-first.json" >/dev/null

curl --fail --silent --show-error -D "$temp_dir/create-first.headers" \
  -b "$cookie_jar" -H 'Content-Type: application/json' -H "Origin: $web_origin" \
  -H 'Idempotency-Key: persistence-restart-contract' \
  --data '{"scenario_id":"checkout-duplicate","fault":"duplicate"}' \
  "$api_origin/v1/runs" >"$temp_dir/create-first.json"
run_id=$(jq -er '.id' "$temp_dir/create-first.json")

deliver_webhook "$temp_dir/webhook-first.json"
jq -e '.received == true and .duplicate == false' "$temp_dir/webhook-first.json" >/dev/null

docker compose -p "$project_name" -f "$compose_file" stop api-test
docker compose -p "$project_name" -f "$compose_file" start api-test
await_api

curl --fail --silent --show-error -b "$cookie_jar" "$api_origin/v1/runs/$stripe_run_id" >"$temp_dir/stripe-run-after-restart.json"
jq -e --arg run_id "$stripe_run_id" '.id == $run_id and (.stripe_object_id | startswith("pi_mock_"))' "$temp_dir/stripe-run-after-restart.json" >/dev/null
create_stripe_run "$connection_id" stripe-payment-restart-contract "$temp_dir/stripe-run-replay.headers" "$temp_dir/stripe-run-replay.json"
tr -d '\r' <"$temp_dir/stripe-run-replay.headers" | grep -qi '^Idempotent-Replayed: true$'
jq -e --arg run_id "$stripe_run_id" '.id == $run_id' "$temp_dir/stripe-run-replay.json" >/dev/null
create_stripe_run "$connection_id" stripe-payment-after-restart-contract "$temp_dir/stripe-run-after-restart-new.headers" "$temp_dir/stripe-run-after-restart-new.json"
jq -e --arg previous_run_id "$stripe_run_id" '
  .id != $previous_run_id and .environment == "sandbox" and (.stripe_object_id | startswith("pi_mock_"))
' "$temp_dir/stripe-run-after-restart-new.json" >/dev/null

curl --fail --silent --show-error -b "$cookie_jar" "$api_origin/v1/runs/$run_id" >"$temp_dir/run-after-restart.json"
jq -e --arg run_id "$run_id" '.id == $run_id and .environment == "sandbox"' "$temp_dir/run-after-restart.json" >/dev/null

curl --fail --silent --show-error -D "$temp_dir/create-replay.headers" \
  -b "$cookie_jar" -H 'Content-Type: application/json' -H "Origin: $web_origin" \
  -H 'Idempotency-Key: persistence-restart-contract' \
  --data '{"scenario_id":"checkout-duplicate","fault":"duplicate"}' \
  "$api_origin/v1/runs" >"$temp_dir/create-replay.json"
tr -d '\r' <"$temp_dir/create-replay.headers" | grep -qi '^Idempotent-Replayed: true$'
jq -e --arg run_id "$run_id" '.id == $run_id' "$temp_dir/create-replay.json" >/dev/null

deliver_webhook "$temp_dir/webhook-second.json"
jq -e '.received == true and .duplicate == true' "$temp_dir/webhook-second.json" >/dev/null

echo "persistence restart contract passed for $run_id"
