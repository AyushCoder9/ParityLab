#!/bin/sh
set -eu

if [ "${PARITYLAB_CONFIRM_FRESH:-0}" != "1" ]; then
  echo "Refusing to recreate the dedicated SSE test database. Re-run with PARITYLAB_CONFIRM_FRESH=1." >&2
  exit 2
fi

for command in awk curl diff docker grep jq openssl sed seq sort uniq wc; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "required command is missing: $command" >&2
    exit 2
  fi
done

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
compose_file="$repo_dir/infra/compose.sse-test.yaml"
project_name=paritylab-sse-reconnect-contract
api_port=${PARITYLAB_SSE_API_PORT:-18088}
api_origin="http://127.0.0.1:${api_port}"
web_origin=${PARITYLAB_SSE_WEB_ORIGIN:-http://127.0.0.1:3206}
webhook_secret=whsec_sse_reconnect_contract
endpoint_token=sse-contract
temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/paritylab-sse.XXXXXX")
auth_password='SSE_QA_NeverLog_42!'
raw_body_sentinel='SSE_RAW_BODY_MUST_NEVER_BE_LOGGED'
initial_stream_pid=
reconnect_stream_pid=

cleanup() {
  status=$?
  trap - EXIT INT TERM
  for stream_pid in ${initial_stream_pid:-} ${reconnect_stream_pid:-}; do
    if [ -n "$stream_pid" ] && kill -0 "$stream_pid" >/dev/null 2>&1; then
      kill "$stream_pid" >/dev/null 2>&1 || true
      wait "$stream_pid" >/dev/null 2>&1 || true
    fi
  done
  if [ "$status" -ne 0 ]; then
    docker compose -p "$project_name" -f "$compose_file" logs api-sse-test worker-sse-test stripe-sse-mock postgres-sse-test >&2 || true
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
      echo "SSE contract API did not become ready" >&2
      return 1
    fi
    sleep 1
  done
}

await_pattern() {
  file=$1
  pattern=$2
  label=$3
  attempt=0
  until grep -F "$pattern" "$file" >/dev/null 2>&1; do
    attempt=$((attempt + 1))
    if [ "$attempt" -ge 90 ]; then
      echo "$label did not appear in $file" >&2
      [ -f "$file" ] && cat "$file" >&2
      return 1
    fi
    sleep 1
  done
}

expect_status() {
  expected=$1
  actual=$2
  body_file=$3
  if [ "$actual" != "$expected" ]; then
    [ -f "$body_file" ] && cat "$body_file" >&2
    echo "expected HTTP $expected, received $actual" >&2
    return 1
  fi
}

register() {
  label=$1
  cookie_jar=$2
  output_file=$3
  status=$(curl --silent --show-error --write-out '%{http_code}' --output "$output_file" -c "$cookie_jar" \
    -H 'Content-Type: application/json' -H "Origin: $web_origin" \
    --data "{\"email\":\"${label}@example.test\",\"password\":\"$auth_password\",\"workspace_name\":\"${label} workspace\",\"project_name\":\"${label} project\"}" \
    "$api_origin/v1/auth/register")
  expect_status 201 "$status" "$output_file"
}

deliver_webhook() {
  body_file=$1
  output_file=$2
  timestamp=$(date +%s)
  signature=$(
    { printf '%s.' "$timestamp"; cat "$body_file"; } |
      openssl dgst -sha256 -hmac "$webhook_secret" -hex | awk '{print $NF}'
  )
  curl --silent --show-error --write-out '%{http_code}' --output "$output_file" \
    -H 'Content-Type: application/json' \
    -H "Stripe-Signature: t=${timestamp},v1=${signature}" \
    --data-binary "@$body_file" \
    "$api_origin/hooks/stripe/$endpoint_token"
}

stop_stream() {
  stream_pid=$1
  if kill -0 "$stream_pid" >/dev/null 2>&1; then
    kill "$stream_pid" >/dev/null 2>&1 || true
  fi
  wait "$stream_pid" >/dev/null 2>&1 || true
}

cd "$repo_dir"
docker compose -p "$project_name" -f "$compose_file" down --volumes --remove-orphans >/dev/null 2>&1 || true
PARITYLAB_SSE_API_PORT="$api_port" PARITYLAB_SSE_WEB_ORIGIN="$web_origin" \
  docker compose -p "$project_name" -f "$compose_file" up -d --wait
await_api

register sse-tenant-a "$temp_dir/a.cookies" "$temp_dir/a-register.json"
connection_status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/connection.json" \
  -b "$temp_dir/a.cookies" -H 'Content-Type: application/json' -H "Origin: $web_origin" \
  --data '{"secret_key":"sk_test_sse_reconnect_contract","sandbox_name":"SSE QA Sandbox"}' \
  "$api_origin/v1/connections/stripe/validate")
expect_status 201 "$connection_status" "$temp_dir/connection.json"
connection_id=$(jq -er '.id' "$temp_dir/connection.json")

run_status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/run.json" \
  -b "$temp_dir/a.cookies" -H 'Content-Type: application/json' -H "Origin: $web_origin" \
  -H 'Idempotency-Key: sse-restart-reconnect-contract' \
  --data "{\"connection_id\":\"$connection_id\",\"amount_minor\":4200,\"currency\":\"usd\"}" \
  "$api_origin/v1/stripe/payment-intents")
expect_status 201 "$run_status" "$temp_dir/run.json"
run_id=$(jq -er '.id' "$temp_dir/run.json")
stripe_object_id=$(jq -er '.stripe_object_id | select(startswith("pi_mock_"))' "$temp_dir/run.json")

curl --fail --silent --show-error -b "$temp_dir/a.cookies" \
  "$api_origin/v1/runs/$run_id/events" >"$temp_dir/events-before.json"
last_sequence=$(jq -er '[.data[].sequence] | max | select(. > 0)' "$temp_dir/events-before.json")
event_count=$(jq -er '.data | length' "$temp_dir/events-before.json")
[ "$event_count" = "$last_sequence" ]
correlation_id=$(jq -er '[.data[].evidence.paritylab_correlation_id | select(type == "string")][0] | select(test("^plcorr_[a-f0-9]{24}$"))' "$temp_dir/events-before.json")

# No cookie cannot cross from the public preview boundary into this tenant run.
anonymous_status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/anonymous.json" \
  -H 'Accept: text/event-stream' "$api_origin/v1/runs/$run_id/events")
expect_status 404 "$anonymous_status" "$temp_dir/anonymous.json"

register sse-tenant-b "$temp_dir/b.cookies" "$temp_dir/b-register.json"
foreign_status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/foreign.json" \
  -b "$temp_dir/b.cookies" -H 'Accept: text/event-stream' "$api_origin/v1/runs/$run_id/events")
expect_status 404 "$foreign_status" "$temp_dir/foreign.json"

invalid_status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/invalid-cursor.json" \
  -b "$temp_dir/a.cookies" -H 'Accept: text/event-stream' -H 'Last-Event-ID: not-a-sequence' \
  "$api_origin/v1/runs/$run_id/events")
expect_status 400 "$invalid_status" "$temp_dir/invalid-cursor.json"
jq -e '.error.code == "invalid_last_event_id" and .error.param == "Last-Event-ID"' "$temp_dir/invalid-cursor.json" >/dev/null

duplicate_cursor_status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/duplicate-cursor.json" \
  -b "$temp_dir/a.cookies" -H 'Accept: text/event-stream' \
  -H "Last-Event-ID: $last_sequence" -H "Last-Event-ID: $last_sequence" \
  "$api_origin/v1/runs/$run_id/events")
expect_status 400 "$duplicate_cursor_status" "$temp_dir/duplicate-cursor.json"
jq -e '.error.code == "invalid_last_event_id" and .error.param == "Last-Event-ID"' "$temp_dir/duplicate-cursor.json" >/dev/null

ahead_sequence=$((last_sequence + 1000))
ahead_status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/ahead-cursor.json" \
  -b "$temp_dir/a.cookies" -H 'Accept: text/event-stream' -H "Last-Event-ID: $ahead_sequence" \
  "$api_origin/v1/runs/$run_id/events")
expect_status 400 "$ahead_status" "$temp_dir/ahead-cursor.json"
jq -e '.error.code == "last_event_id_ahead" and .error.param == "Last-Event-ID"' "$temp_dir/ahead-cursor.json" >/dev/null

curl --no-buffer --silent --show-error --max-time 60 \
  -D "$temp_dir/initial.headers" -b "$temp_dir/a.cookies" \
  -H 'Accept: text/event-stream' \
  "$api_origin/v1/runs/$run_id/events" \
  >"$temp_dir/initial.sse" 2>"$temp_dir/initial.stderr" &
initial_stream_pid=$!
await_pattern "$temp_dir/initial.headers" 'HTTP/1.1 200 OK' "initial SSE status"
await_pattern "$temp_dir/initial.headers" 'Content-Type: text/event-stream' "initial SSE content type"
await_pattern "$temp_dir/initial.sse" 'retry: 2000' "SSE retry directive"
await_pattern "$temp_dir/initial.sse" "id: $last_sequence" "initial event replay"
await_pattern "$temp_dir/initial.sse" ': heartbeat' "initial SSE heartbeat"
kill -0 "$initial_stream_pid"
stop_stream "$initial_stream_pid"
initial_stream_pid=

grep '^id: ' "$temp_dir/initial.sse" | sed 's/^id: //' >"$temp_dir/initial.ids"
[ "$(wc -l <"$temp_dir/initial.ids" | tr -d ' ')" = "$event_count" ]
seq 1 "$last_sequence" >"$temp_dir/expected.ids"
diff -u "$temp_dir/expected.ids" "$temp_dir/initial.ids"
[ "$(grep -c '^retry: 2000$' "$temp_dir/initial.sse")" = "1" ]
[ "$(grep -c '^: heartbeat$' "$temp_dir/initial.sse")" -ge 1 ]
[ "$(grep -c '^event: run.complete$' "$temp_dir/initial.sse")" = "1" ]
grep '^data: ' "$temp_dir/initial.sse" | sed 's/^data: //' |
  while IFS= read -r payload; do
    printf '%s\n' "$payload" | jq -e . >/dev/null
  done

# The API process disappears, but the cursor and event ledger remain durable.
docker compose -p "$project_name" -f "$compose_file" stop api-sse-test
docker compose -p "$project_name" -f "$compose_file" start api-sse-test
await_api

curl --no-buffer --silent --show-error --max-time 60 \
  -D "$temp_dir/reconnect.headers" -b "$temp_dir/a.cookies" \
  -H 'Accept: text/event-stream' -H "Last-Event-ID: $last_sequence" \
  "$api_origin/v1/runs/$run_id/events" \
  >"$temp_dir/reconnect.sse" 2>"$temp_dir/reconnect.stderr" &
reconnect_stream_pid=$!
await_pattern "$temp_dir/reconnect.headers" 'HTTP/1.1 200 OK' "reconnected SSE status"
await_pattern "$temp_dir/reconnect.sse" 'retry: 2000' "reconnected retry directive"
await_pattern "$temp_dir/reconnect.sse" ': heartbeat' "reconnected SSE heartbeat"
kill -0 "$reconnect_stream_pid"
[ "$(grep -c '^id: ' "$temp_dir/reconnect.sse" || true)" = "0" ]

jq -nc \
  --arg object_id "$stripe_object_id" \
  --arg correlation "$correlation_id" \
  --arg sentinel "$raw_body_sentinel" \
  '{id:"evt_sse_live_append",object:"event",type:"payment_intent.succeeded",created:1784736004,livemode:false,data:{object:{id:$object_id,object:"payment_intent",status:"succeeded",metadata:{paritylab_correlation_id:$correlation,private_note:$sentinel}}}}' \
  >"$temp_dir/webhook-body.json"
webhook_status=$(deliver_webhook "$temp_dir/webhook-body.json" "$temp_dir/webhook.json")
expect_status 200 "$webhook_status" "$temp_dir/webhook.json"
jq -e '.received == true and .duplicate == false' "$temp_dir/webhook.json" >/dev/null

next_sequence=$((last_sequence + 1))
await_pattern "$temp_dir/reconnect.sse" "id: $next_sequence" "live webhook-correlated SSE append"
kill -0 "$reconnect_stream_pid"
stop_stream "$reconnect_stream_pid"
reconnect_stream_pid=

grep '^id: ' "$temp_dir/reconnect.sse" | sed 's/^id: //' >"$temp_dir/reconnect.ids"
[ "$(wc -l <"$temp_dir/reconnect.ids" | tr -d ' ')" = "1" ]
[ "$(cat "$temp_dir/reconnect.ids")" = "$next_sequence" ]
[ "$(grep -c '^retry: 2000$' "$temp_dir/reconnect.sse")" = "1" ]
[ "$(grep -c '^: heartbeat$' "$temp_dir/reconnect.sse")" -ge 1 ]
[ "$(grep -c '^event: run.complete$' "$temp_dir/reconnect.sse")" = "1" ]
grep -F '"stripe_event_id":"evt_sse_live_append"' "$temp_dir/reconnect.sse" >/dev/null
grep -F '"checkpoint":"stripe-webhook"' "$temp_dir/reconnect.sse" >/dev/null
! grep -F "$raw_body_sentinel" "$temp_dir/reconnect.sse" >/dev/null

cat "$temp_dir/initial.ids" "$temp_dir/reconnect.ids" >"$temp_dir/all.ids"
[ "$(sort -n "$temp_dir/all.ids" | uniq -d | wc -l | tr -d ' ')" = "0" ]
seq 1 "$next_sequence" >"$temp_dir/all-expected.ids"
diff -u "$temp_dir/all-expected.ids" "$temp_dir/all.ids"

curl --fail --silent --show-error -b "$temp_dir/a.cookies" \
  "$api_origin/v1/runs/$run_id/events" >"$temp_dir/events-after.json"
jq -e --argjson sequence "$next_sequence" '
  (.data | length) == $sequence and
  ([.data[] | select(
    .sequence == $sequence and
    .checkpoint == "stripe-webhook" and
    .evidence.stripe_event_id == "evt_sse_live_append"
  )] | length) == 1
' "$temp_dir/events-after.json" >/dev/null

docker compose -p "$project_name" -f "$compose_file" logs >"$temp_dir/services.log"
! grep -F "$raw_body_sentinel" "$temp_dir/services.log" >/dev/null
! grep -F "$webhook_secret" "$temp_dir/services.log" >/dev/null
! grep -F "$auth_password" "$temp_dir/services.log" >/dev/null
! grep -F 'sk_test_sse_reconnect_contract' "$temp_dir/services.log" >/dev/null

echo "SSE restart and reconnect contract passed for $run_id through sequence $next_sequence"
