#!/bin/sh
set -eu

if [ "${PARITYLAB_CONFIRM_FRESH:-0}" != "1" ]; then
  echo "Refusing to recreate the dedicated webhook-consumer database. Re-run with PARITYLAB_CONFIRM_FRESH=1." >&2
  exit 2
fi

for command in curl docker jq openssl; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "required command is missing: $command" >&2
    exit 2
  fi
done

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
compose_file="$repo_dir/infra/compose.webhook-test.yaml"
project_name=paritylab-webhook-consumer-contract
api_port=${PARITYLAB_WEBHOOK_API_PORT:-18086}
api_origin="http://127.0.0.1:${api_port}"
web_origin=${PARITYLAB_WEBHOOK_WEB_ORIGIN:-http://127.0.0.1:3204}
webhook_secret=whsec_webhook_consumer_contract
endpoint_token=webhook-contract
temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/paritylab-webhook.XXXXXX")
auth_password='Webhook_QA_NeverLog_42!'
raw_body_sentinel='RAW_WEBHOOK_BODY_MUST_NEVER_BE_STORED_OR_LOGGED'

cleanup() {
  status=$?
  trap - EXIT INT TERM
  if [ "$status" -ne 0 ]; then
    docker compose -p "$project_name" -f "$compose_file" logs api-webhook-test worker-webhook-test stripe-webhook-mock postgres-webhook-test >&2 || true
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
    if [ "$attempt" -ge 120 ]; then
      echo "webhook contract API did not become ready" >&2
      return 1
    fi
    sleep 1
  done
}

db_scalar() {
  docker compose -p "$project_name" -f "$compose_file" exec -T postgres-webhook-test \
    psql -U paritylab -d paritylab -v ON_ERROR_STOP=1 -At -c "$1" | head -1
}

await_scalar() {
  expected=$1
  sql=$2
  label=$3
  attempt=0
  while :; do
    actual=$(db_scalar "$sql")
    if [ "$actual" = "$expected" ]; then
      return 0
    fi
    attempt=$((attempt + 1))
    if [ "$attempt" -ge 90 ]; then
      echo "$label did not converge: expected '$expected', received '$actual'" >&2
      return 1
    fi
    sleep 1
  done
}

deliver_webhook() {
  body_file=$1
  output_file=$2
  headers_file=$3
  timestamp=$(date +%s)
  signature=$(
    { printf '%s.' "$timestamp"; cat "$body_file"; } |
      openssl dgst -sha256 -hmac "$webhook_secret" -hex | awk '{print $NF}'
  )
  curl --silent --show-error --write-out '%{http_code}' --output "$output_file" -D "$headers_file" \
    -H 'Content-Type: application/json' \
    -H "Stripe-Signature: t=${timestamp},v1=${signature}" \
    --data-binary "@$body_file" \
    "$api_origin/hooks/stripe/$endpoint_token"
}

expect_status() {
  expected=$1
  actual=$2
  body_file=$3
  if [ "$actual" != "$expected" ]; then
    cat "$body_file" >&2
    echo "expected HTTP $expected, received $actual" >&2
    return 1
  fi
}

cd "$repo_dir"
docker compose -p "$project_name" -f "$compose_file" down --volumes --remove-orphans >/dev/null 2>&1 || true
PARITYLAB_WEBHOOK_API_PORT="$api_port" PARITYLAB_WEBHOOK_WEB_ORIGIN="$web_origin" \
  docker compose -p "$project_name" -f "$compose_file" up -d --wait \
  postgres-webhook-test stripe-webhook-mock api-webhook-test
await_api

register_status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/register.json" \
  -D "$temp_dir/register.headers" -c "$temp_dir/session.cookies" \
  -H 'Content-Type: application/json' -H "Origin: $web_origin" \
  --data "{\"email\":\"webhook-owner@example.test\",\"password\":\"$auth_password\",\"workspace_name\":\"Webhook QA\",\"project_name\":\"Webhook consumer contract\"}" \
  "$api_origin/v1/auth/register")
expect_status 201 "$register_status" "$temp_dir/register.json"
project_id=$(jq -er '.project.id' "$temp_dir/register.json")

connection_status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/connection.json" \
  -b "$temp_dir/session.cookies" -H 'Content-Type: application/json' -H "Origin: $web_origin" \
  --data '{"secret_key":"sk_test_webhook_consumer_contract","sandbox_name":"Webhook QA Sandbox"}' \
  "$api_origin/v1/connections/stripe/validate")
expect_status 201 "$connection_status" "$temp_dir/connection.json"
connection_id=$(jq -er '.id' "$temp_dir/connection.json")

run_status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/run.json" \
  -b "$temp_dir/session.cookies" -H 'Content-Type: application/json' -H "Origin: $web_origin" \
  -H 'Idempotency-Key: webhook-consumer-correlation-contract' \
  --data "{\"connection_id\":\"$connection_id\",\"amount_minor\":4200,\"currency\":\"usd\"}" \
  "$api_origin/v1/stripe/payment-intents")
expect_status 201 "$run_status" "$temp_dir/run.json"
run_id=$(jq -er '.id' "$temp_dir/run.json")
stripe_object_id=$(jq -er '.stripe_object_id | select(startswith("pi_mock_"))' "$temp_dir/run.json")

curl --fail --silent --show-error -b "$temp_dir/session.cookies" \
  "$api_origin/v1/runs/$run_id/events" >"$temp_dir/run-events-before.json"
correlation_id=$(jq -er '[.data[].evidence.paritylab_correlation_id | select(type == "string")][0] | select(test("^plcorr_[a-f0-9]{24}$"))' "$temp_dir/run-events-before.json")
events_before=$(jq -er '.data | length' "$temp_dir/run-events-before.json")
curl --fail --silent --show-error -b "$temp_dir/session.cookies" \
  "$api_origin/v1/runs/$run_id/report" >"$temp_dir/report-before.json"

jq -nc \
  --arg id evt_webhook_consumer_matched \
  --arg object_id "$stripe_object_id" \
  --arg correlation "$correlation_id" \
  --arg sentinel "$raw_body_sentinel" \
  '{id:$id,object:"event",type:"payment_intent.succeeded",created:1784736000,livemode:false,data:{object:{id:$object_id,object:"payment_intent",status:"succeeded",metadata:{paritylab_correlation_id:$correlation,private_note:$sentinel},billing_details:{email:"never-store@example.test"}}}}' \
  >"$temp_dir/matched-body.json"

matched_status=$(deliver_webhook "$temp_dir/matched-body.json" "$temp_dir/matched-first.json" "$temp_dir/matched-first.headers")
expect_status 200 "$matched_status" "$temp_dir/matched-first.json"
jq -e '.received == true and .duplicate == false and .event.id == "evt_webhook_consumer_matched"' "$temp_dir/matched-first.json" >/dev/null

# Ingress is atomic and durable before a consumer exists. It stores only hashes
# plus the frozen sanitized projection; the raw signed body remains absent.
[ "$(db_scalar "SELECT processing_status FROM webhook_events WHERE stripe_event_id='evt_webhook_consumer_matched'")" = "pending" ]
[ "$(db_scalar "SELECT count(*) FROM webhook_events WHERE stripe_event_id='evt_webhook_consumer_matched' AND body_ciphertext IS NULL AND stripe_object_id='$stripe_object_id' AND paritylab_correlation_id='$correlation_id'")" = "1" ]
[ "$(db_scalar "SELECT count(*) FROM outbox WHERE aggregate_id='evt_webhook_consumer_matched' AND topic='stripe.webhook.received' AND published_at IS NULL AND failed_at IS NULL")" = "1" ]

# Prove ingress survives an API process restart before the worker claims it.
docker compose -p "$project_name" -f "$compose_file" stop api-webhook-test
docker compose -p "$project_name" -f "$compose_file" start api-webhook-test
await_api
[ "$(db_scalar "SELECT processing_status FROM webhook_events WHERE stripe_event_id='evt_webhook_consumer_matched'")" = "pending" ]

docker compose -p "$project_name" -f "$compose_file" up -d worker-webhook-test
await_scalar matched "SELECT processing_status FROM webhook_events WHERE stripe_event_id='evt_webhook_consumer_matched'" "matched webhook processing"

[ "$(db_scalar "SELECT count(*) FROM webhook_events WHERE stripe_event_id='evt_webhook_consumer_matched' AND correlated_run_id='$run_id' AND correlated_project_id='$project_id'::uuid AND processed_at IS NOT NULL AND processing_error_code IS NULL")" = "1" ]
[ "$(db_scalar "SELECT count(*) FROM stripe_webhook_evidence WHERE stripe_event_id='evt_webhook_consumer_matched' AND run_id='$run_id' AND project_id='$project_id'::uuid AND stripe_object_id='$stripe_object_id' AND paritylab_correlation_id='$correlation_id'")" = "1" ]
[ "$(db_scalar "SELECT count(*) FROM outbox WHERE aggregate_id='evt_webhook_consumer_matched' AND topic='stripe.webhook.received' AND published_at IS NOT NULL AND failed_at IS NULL")" = "1" ]

curl --fail --silent --show-error -b "$temp_dir/session.cookies" \
  "$api_origin/v1/runs/$run_id/events" >"$temp_dir/run-events-matched.json"
events_matched=$(jq -er '.data | length' "$temp_dir/run-events-matched.json")
[ "$events_matched" -eq $((events_before + 1)) ]
jq -e --arg object_id "$stripe_object_id" --arg correlation "$correlation_id" '
  [.data[] | select(
    .evidence.stripe_event_id == "evt_webhook_consumer_matched" and
    .evidence.stripe_event_type == "payment_intent.succeeded" and
    .evidence.stripe_payment_intent_id == $object_id and
    .evidence.stripe_payment_intent_status == "succeeded" and
    .evidence.paritylab_correlation_id == $correlation and
    .checkpoint == "stripe-webhook"
  )] | length == 1
' "$temp_dir/run-events-matched.json" >/dev/null
! grep -F "$raw_body_sentinel" "$temp_dir/run-events-matched.json" >/dev/null
! grep -F 'never-store@example.test' "$temp_dir/run-events-matched.json" >/dev/null
curl --fail --silent --show-error -b "$temp_dir/session.cookies" \
  "$api_origin/v1/runs/$run_id/report" >"$temp_dir/report-matched.json"
jq -e '[.assertions[] | select(.id == "assert_stripe_webhook_correlated" and .passed == true)] | length == 1' "$temp_dir/report-matched.json" >/dev/null

# Replay after a worker restart is acknowledged at ingress without a second
# outbox message, evidence row, run event, or report assertion.
docker compose -p "$project_name" -f "$compose_file" stop worker-webhook-test
replay_status=$(deliver_webhook "$temp_dir/matched-body.json" "$temp_dir/matched-replay.json" "$temp_dir/matched-replay.headers")
expect_status 200 "$replay_status" "$temp_dir/matched-replay.json"
jq -e '.received == true and .duplicate == true' "$temp_dir/matched-replay.json" >/dev/null
docker compose -p "$project_name" -f "$compose_file" start worker-webhook-test
sleep 2
[ "$(db_scalar "SELECT count(*) FROM stripe_webhook_evidence WHERE stripe_event_id='evt_webhook_consumer_matched'")" = "1" ]
[ "$(db_scalar "SELECT count(*) FROM outbox WHERE aggregate_id='evt_webhook_consumer_matched' AND topic='stripe.webhook.received'")" = "1" ]
curl --fail --silent --show-error -b "$temp_dir/session.cookies" \
  "$api_origin/v1/runs/$run_id/events" >"$temp_dir/run-events-replay.json"
[ "$(jq -er '.data | length' "$temp_dir/run-events-replay.json")" = "$events_matched" ]
curl --fail --silent --show-error -b "$temp_dir/session.cookies" \
  "$api_origin/v1/runs/$run_id/report" >"$temp_dir/report-replay.json"
jq -e '[.assertions[] | select(.id == "assert_stripe_webhook_correlated")] | length == 1' "$temp_dir/report-replay.json" >/dev/null

jq '.data.object.status = "requires_payment_method"' "$temp_dir/matched-body.json" >"$temp_dir/conflict-body.json"
conflict_status=$(deliver_webhook "$temp_dir/conflict-body.json" "$temp_dir/conflict.json" "$temp_dir/conflict.headers")
expect_status 400 "$conflict_status" "$temp_dir/conflict.json"
jq -e '.error.code == "webhook_event_conflict"' "$temp_dir/conflict.json" >/dev/null
[ "$(db_scalar "SELECT count(*) FROM stripe_webhook_evidence WHERE stripe_event_id='evt_webhook_consumer_matched'")" = "1" ]

jq -nc \
  --arg object_id "$stripe_object_id" \
  --arg correlation "$correlation_id" \
  '{id:"evt_webhook_consumer_ignored",object:"event",type:"payment_intent.future_state",created:1784736001,livemode:false,data:{object:{id:$object_id,object:"payment_intent",status:"future_state",metadata:{paritylab_correlation_id:$correlation}}}}' \
  >"$temp_dir/ignored-body.json"
ignored_status=$(deliver_webhook "$temp_dir/ignored-body.json" "$temp_dir/ignored.json" "$temp_dir/ignored.headers")
expect_status 200 "$ignored_status" "$temp_dir/ignored.json"
await_scalar ignored "SELECT processing_status FROM webhook_events WHERE stripe_event_id='evt_webhook_consumer_ignored'" "ignored webhook processing"
[ "$(db_scalar "SELECT count(*) FROM webhook_events WHERE stripe_event_id='evt_webhook_consumer_ignored' AND processed_at IS NOT NULL AND processing_error_code='unsupported_event_type' AND correlated_run_id IS NULL AND correlated_project_id IS NULL")" = "1" ]
[ "$(db_scalar "SELECT count(*) FROM outbox WHERE aggregate_id='evt_webhook_consumer_ignored' AND published_at IS NOT NULL AND failed_at IS NULL")" = "1" ]

jq -nc \
  '{id:"evt_webhook_consumer_unmatched",object:"event",type:"payment_intent.succeeded",created:1784736002,livemode:false,data:{object:{id:"pi_unmatched_contract",object:"payment_intent",status:"succeeded",metadata:{paritylab_correlation_id:"plcorr_aaaaaaaaaaaaaaaaaaaaaaaa"}}}}' \
  >"$temp_dir/unmatched-body.json"
unmatched_status=$(deliver_webhook "$temp_dir/unmatched-body.json" "$temp_dir/unmatched.json" "$temp_dir/unmatched.headers")
expect_status 200 "$unmatched_status" "$temp_dir/unmatched.json"
await_scalar unmatched "SELECT processing_status FROM webhook_events WHERE stripe_event_id='evt_webhook_consumer_unmatched'" "unmatched webhook processing"
[ "$(db_scalar "SELECT count(*) FROM webhook_events WHERE stripe_event_id='evt_webhook_consumer_unmatched' AND processed_at IS NOT NULL AND processing_error_code='run_not_found' AND correlated_run_id IS NULL AND correlated_project_id IS NULL")" = "1" ]
[ "$(db_scalar "SELECT count(*) FROM outbox WHERE aggregate_id='evt_webhook_consumer_unmatched' AND published_at IS NOT NULL AND failed_at IS NULL")" = "1" ]

# A real object ID is insufficient without ParityLab's unguessable correlation
# value. This prevents object-only cross-tenant attribution.
jq -nc \
  --arg object_id "$stripe_object_id" \
  '{id:"evt_webhook_consumer_missing_correlation",object:"event",type:"payment_intent.succeeded",created:1784736003,livemode:false,data:{object:{id:$object_id,object:"payment_intent",status:"succeeded",metadata:{}}}}' \
  >"$temp_dir/missing-correlation-body.json"
missing_correlation_status=$(deliver_webhook "$temp_dir/missing-correlation-body.json" "$temp_dir/missing-correlation.json" "$temp_dir/missing-correlation.headers")
expect_status 200 "$missing_correlation_status" "$temp_dir/missing-correlation.json"
await_scalar unmatched "SELECT processing_status FROM webhook_events WHERE stripe_event_id='evt_webhook_consumer_missing_correlation'" "missing-correlation webhook processing"
[ "$(db_scalar "SELECT count(*) FROM webhook_events WHERE stripe_event_id='evt_webhook_consumer_missing_correlation' AND processed_at IS NOT NULL AND processing_error_code='missing_correlation_id' AND correlated_run_id IS NULL AND correlated_project_id IS NULL")" = "1" ]
[ "$(db_scalar "SELECT count(*) FROM outbox WHERE aggregate_id='evt_webhook_consumer_missing_correlation' AND published_at IS NOT NULL AND failed_at IS NULL")" = "1" ]
[ "$(db_scalar "SELECT count(*) FROM stripe_webhook_evidence WHERE stripe_event_id IN ('evt_webhook_consumer_ignored','evt_webhook_consumer_unmatched','evt_webhook_consumer_missing_correlation')")" = "0" ]

# A malformed internal job is poison data, not a retry loop.
malformed_outbox_id=$(db_scalar "INSERT INTO outbox (aggregate_type,aggregate_id,topic,payload) VALUES ('stripe_event','evt_malformed_internal_job','stripe.webhook.received','{}'::jsonb) RETURNING id")
await_scalar webhook_job_invalid "SELECT COALESCE(last_error_code,'') FROM outbox WHERE id='$malformed_outbox_id'::uuid AND failed_at IS NOT NULL" "malformed webhook job terminal failure"
[ "$(db_scalar "SELECT count(*) FROM outbox WHERE id='$malformed_outbox_id'::uuid AND published_at IS NULL AND failed_at IS NOT NULL AND attempts=1")" = "1" ]

docker compose -p "$project_name" -f "$compose_file" logs >"$temp_dir/service.log"
! grep -F "$raw_body_sentinel" "$temp_dir/service.log" >/dev/null
! grep -F "$webhook_secret" "$temp_dir/service.log" >/dev/null
! grep -F "$auth_password" "$temp_dir/service.log" >/dev/null
! grep -F 'sk_test_webhook_consumer_contract' "$temp_dir/service.log" >/dev/null

echo "webhook consumer restart contract passed for $run_id"
