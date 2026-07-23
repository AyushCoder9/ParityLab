#!/bin/sh
set -eu

if [ "${PARITYLAB_CONFIRM_FRESH:-0}" != "1" ]; then
  echo "Refusing to recreate the dedicated auth test database. Re-run with PARITYLAB_CONFIRM_FRESH=1." >&2
  exit 2
fi

for command in curl docker jq openssl; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "required command is missing: $command" >&2
    exit 2
  fi
done

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
compose_file="$repo_dir/infra/compose.auth-test.yaml"
project_name=paritylab-auth-security-contract
api_port=${PARITYLAB_AUTH_API_PORT:-18084}
api_origin="http://127.0.0.1:${api_port}"
web_origin=${PARITYLAB_AUTH_WEB_ORIGIN:-http://127.0.0.1:3202}
temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/paritylab-auth.XXXXXX")
auth_password='QA_LOG_SENTINEL_NeverLog_42!'

cleanup() {
  status=$?
  trap - EXIT INT TERM
  if [ "$status" -ne 0 ]; then
    docker compose -p "$project_name" -f "$compose_file" logs api-auth-test postgres-auth-test >&2 || true
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
      echo "auth API did not become ready" >&2
      return 1
    fi
    sleep 1
  done
}

register() {
  label=$1
  cookie_jar=$2
  headers=$3
  body=$4
  status=$(curl --silent --show-error --write-out '%{http_code}' --output "$body" -D "$headers" -c "$cookie_jar" \
    -H 'Content-Type: application/json' -H "Origin: $web_origin" \
    --data "{\"email\":\"${label}@example.test\",\"password\":\"${auth_password}\",\"workspace_name\":\"${label} workspace\",\"project_name\":\"${label} project\"}" \
    "$api_origin/v1/auth/register")
  [ "$status" = "201" ] || { cat "$body" >&2; echo "registration returned HTTP $status" >&2; return 1; }
  ! grep -F "$auth_password" "$body" >/dev/null
  jq -e --arg email "${label}@example.test" --arg workspace "${label} workspace" --arg project "${label} project" '
    .authenticated == true and .user.email == $email and
    .organization.name == $workspace and .organization.role == "owner" and
    .project.name == $project and .project.retention_days == 30 and
    (.expires_at | type == "string") and
    (.password == null) and (.password_hash == null)
  ' "$body" >/dev/null
}

expect_status() {
  expected=$1
  actual=$2
  body=$3
  if [ "$actual" != "$expected" ]; then
    cat "$body" >&2
    echo "expected HTTP $expected, received $actual" >&2
    return 1
  fi
}

cd "$repo_dir"
docker compose -p "$project_name" -f "$compose_file" down --volumes --remove-orphans >/dev/null 2>&1 || true

# Production/default behavior emits a Secure cookie. This header-only phase does
# not attempt to replay that cookie over loopback HTTP.
PARITYLAB_AUTH_API_PORT="$api_port" PARITYLAB_AUTH_WEB_ORIGIN="$web_origin" PARITYLAB_INSECURE_COOKIES=false \
  docker compose -p "$project_name" -f "$compose_file" up -d --wait
await_api
register secure-cookie "$temp_dir/secure.cookies" "$temp_dir/secure.headers" "$temp_dir/secure.json"
secure_header=$(tr -d '\r' <"$temp_dir/secure.headers" | grep -Ei '^Set-Cookie: paritylab_session=' | head -1)
printf '%s' "$secure_header" | grep -Eqi '(; |^)Path=/'
printf '%s' "$secure_header" | grep -Eqi '(; |^)Max-Age=86400(;|$)'
printf '%s' "$secure_header" | grep -Eqi '(; |^)HttpOnly(;|$)'
printf '%s' "$secure_header" | grep -Eqi '(; |^)Secure(;|$)'
printf '%s' "$secure_header" | grep -Eqi '(; |^)SameSite=Lax(;|$)'
# Recreate the API with the loopback cookie policy while retaining the dedicated
# database and Go caches. Final cleanup still removes every scoped volume.
docker compose -p "$project_name" -f "$compose_file" down --remove-orphans >/dev/null

# The browser phase explicitly opts into loopback-only insecure cookies.
PARITYLAB_AUTH_API_PORT="$api_port" PARITYLAB_AUTH_WEB_ORIGIN="$web_origin" PARITYLAB_INSECURE_COOKIES=true \
  docker compose -p "$project_name" -f "$compose_file" up -d --wait
await_api

anonymous_connection_status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/anonymous-connection.json" \
  -H 'Content-Type: application/json' -H "Origin: $web_origin" \
  --data '{"secret_key":"sk_test_must_not_reach_stripe","sandbox_name":"Anonymous"}' \
  "$api_origin/v1/connections/stripe/validate")
expect_status 401 "$anonymous_connection_status" "$temp_dir/anonymous-connection.json"
anonymous_run_status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/anonymous-run.json" \
  -H 'Content-Type: application/json' -H "Origin: $web_origin" \
  -H 'Idempotency-Key: anonymous-must-not-run' \
  --data '{"connection_id":"conn_other_tenant","amount_minor":4200,"currency":"usd"}' \
  "$api_origin/v1/stripe/payment-intents")
expect_status 401 "$anonymous_run_status" "$temp_dir/anonymous-run.json"

register tenant-a "$temp_dir/a.cookies" "$temp_dir/a.headers" "$temp_dir/a-register.json"
insecure_header=$(tr -d '\r' <"$temp_dir/a.headers" | grep -Ei '^Set-Cookie: paritylab_session=' | head -1)
printf '%s' "$insecure_header" | grep -Eqi '(; |^)Path=/'
printf '%s' "$insecure_header" | grep -Eqi '(; |^)Max-Age=86400(;|$)'
printf '%s' "$insecure_header" | grep -Eqi '(; |^)HttpOnly(;|$)'
printf '%s' "$insecure_header" | grep -Eqi '(; |^)SameSite=Lax(;|$)'
! printf '%s' "$insecure_header" | grep -Eqi '(; |^)Secure(;|$)'

known_login_status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/known-login-failure.json" \
  -H 'Content-Type: application/json' -H "Origin: $web_origin" \
  --data '{"email":"tenant-a@example.test","password":"incorrect-password"}' \
  "$api_origin/v1/auth/login")
expect_status 401 "$known_login_status" "$temp_dir/known-login-failure.json"
unknown_login_status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/unknown-login-failure.json" \
  -H 'Content-Type: application/json' -H "Origin: $web_origin" \
  --data '{"email":"absent-account@example.test","password":"incorrect-password"}' \
  "$api_origin/v1/auth/login")
expect_status 401 "$unknown_login_status" "$temp_dir/unknown-login-failure.json"
jq -e '.error.code == "invalid_credentials"' "$temp_dir/known-login-failure.json" >/dev/null
[ "$(jq -cS '.error | {type,code,message,param}' "$temp_dir/known-login-failure.json")" = "$(jq -cS '.error | {type,code,message,param}' "$temp_dir/unknown-login-failure.json")" ]

rate_limited=0
attempt=0
while [ "$attempt" -lt 12 ]; do
  attempt=$((attempt + 1))
  throttle_status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/throttled-login.json" \
    -D "$temp_dir/throttled-login.headers" -H 'Content-Type: application/json' -H "Origin: $web_origin" \
    --data '{"email":"tenant-a@example.test","password":"incorrect-password"}' \
    "$api_origin/v1/auth/login")
  if [ "$throttle_status" = "429" ]; then
    rate_limited=1
    break
  fi
  expect_status 401 "$throttle_status" "$temp_dir/throttled-login.json"
done
[ "$rate_limited" = "1" ]
jq -e '.error.code == "rate_limit_exceeded"' "$temp_dir/throttled-login.json" >/dev/null
tr -d '\r' <"$temp_dir/throttled-login.headers" | grep -Eqi '^Retry-After: [0-9]+$'

settings_status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/settings.json" \
  -b "$temp_dir/a.cookies" -H 'Content-Type: application/json' -H "Origin: $web_origin" \
  -X PATCH --data '{"name":"Persisted QA project","retention_days":90}' "$api_origin/v1/settings/project")
expect_status 200 "$settings_status" "$temp_dir/settings.json"
jq -e '.name == "Persisted QA project" and .retention_days == 90' "$temp_dir/settings.json" >/dev/null

curl --fail --silent --show-error -b "$temp_dir/a.cookies" "$api_origin/v1/environments" >"$temp_dir/environments.json"
environment_id=$(jq -er '.data[] | select(.kind == "staging") | .id' "$temp_dir/environments.json")
select_status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/environment-selected.json" \
  -b "$temp_dir/a.cookies" -H "Origin: $web_origin" -X POST "$api_origin/v1/environments/$environment_id/select")
expect_status 200 "$select_status" "$temp_dir/environment-selected.json"
jq -e '.id == $id and .is_default == true' --arg id "$environment_id" "$temp_dir/environment-selected.json" >/dev/null

connection_status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/connection.json" \
  -b "$temp_dir/a.cookies" -H 'Content-Type: application/json' -H "Origin: $web_origin" \
  --data '{"secret_key":"sk_test_auth_resource_contract","sandbox_name":"Auth QA Sandbox"}' \
  "$api_origin/v1/connections/stripe/validate")
expect_status 201 "$connection_status" "$temp_dir/connection.json"
connection_id=$(jq -er '.id' "$temp_dir/connection.json")
! jq -e 'has("secret_key") or has("ciphertext")' "$temp_dir/connection.json" >/dev/null

run_status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/run.json" \
  -b "$temp_dir/a.cookies" -H 'Content-Type: application/json' -H "Origin: $web_origin" \
  -H 'Idempotency-Key: auth-resource-restart-contract' \
  --data "{\"connection_id\":\"$connection_id\",\"amount_minor\":4200,\"currency\":\"usd\"}" \
  "$api_origin/v1/stripe/payment-intents")
expect_status 201 "$run_status" "$temp_dir/run.json"

curl --fail --silent --show-error -b "$temp_dir/a.cookies" "$api_origin/v1/findings?status=all" >"$temp_dir/findings.json"
finding_id=$(jq -er '.data[0].id' "$temp_dir/findings.json")
resolve_status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/finding-resolved.json" \
  -b "$temp_dir/a.cookies" -H "Origin: $web_origin" -X POST "$api_origin/v1/findings/$finding_id/resolve")
expect_status 200 "$resolve_status" "$temp_dir/finding-resolved.json"
jq -e '.resolved == true' "$temp_dir/finding-resolved.json" >/dev/null

curl --fail --silent --show-error -b "$temp_dir/a.cookies" "$api_origin/v1/notifications" >"$temp_dir/notifications.json"
notification_id=$(jq -er '.data[0].id' "$temp_dir/notifications.json")
read_status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/notification-read.json" \
  -b "$temp_dir/a.cookies" -H "Origin: $web_origin" -X POST "$api_origin/v1/notifications/$notification_id/read")
expect_status 200 "$read_status" "$temp_dir/notification-read.json"
jq -e '.read_at | type == "string"' "$temp_dir/notification-read.json" >/dev/null

register tenant-b "$temp_dir/b.cookies" "$temp_dir/b.headers" "$temp_dir/b-register.json"
cross_connection_status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/cross-tenant-connection.json" \
  -b "$temp_dir/b.cookies" -H 'Content-Type: application/json' -H "Origin: $web_origin" \
  -H 'Idempotency-Key: cross-tenant-connection-must-not-run' \
  --data "{\"connection_id\":\"$connection_id\",\"amount_minor\":4200,\"currency\":\"usd\"}" \
  "$api_origin/v1/stripe/payment-intents")
expect_status 404 "$cross_connection_status" "$temp_dir/cross-tenant-connection.json"
for target in \
  "$api_origin/v1/environments/$environment_id/select" \
  "$api_origin/v1/findings/$finding_id/resolve" \
  "$api_origin/v1/notifications/$notification_id/read"; do
  cross_status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/cross-tenant.json" \
    -b "$temp_dir/b.cookies" -H "Origin: $web_origin" -X POST "$target")
  expect_status 404 "$cross_status" "$temp_dir/cross-tenant.json"
done

csrf_status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/csrf.json" \
  -b "$temp_dir/a.cookies" -H 'Content-Type: application/json' -H 'Origin: https://attacker.invalid' \
  -X PATCH --data '{"name":"must not persist"}' "$api_origin/v1/settings/project")
expect_status 403 "$csrf_status" "$temp_dir/csrf.json"
jq -e '.error.code == "csrf_origin_invalid"' "$temp_dir/csrf.json" >/dev/null

b_token=$(tr -d '\r' <"$temp_dir/b.headers" | sed -n 's/^Set-Cookie: paritylab_session=\([^;]*\).*/\1/p' | head -1)
b_hash=$(printf '%s' "$b_token" | openssl dgst -sha256 -hex | awk '{print $NF}')
case "$b_hash" in
  (*[!0-9a-f]*|"") echo "invalid session hash" >&2; exit 1 ;;
esac
[ "${#b_hash}" -eq 64 ]
expired=$(docker compose -p "$project_name" -f "$compose_file" exec -T postgres-auth-test \
  psql -U paritylab -d paritylab -At \
  -c "UPDATE sessions SET expires_at=now()-interval '1 minute' WHERE token_hash=decode('$b_hash','hex') RETURNING 1" | head -1)
[ "$expired" = "1" ]
expired_status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/expired.json" \
  -b "$temp_dir/b.cookies" "$api_origin/v1/session")
expect_status 401 "$expired_status" "$temp_dir/expired.json"

docker compose -p "$project_name" -f "$compose_file" stop api-auth-test
docker compose -p "$project_name" -f "$compose_file" start api-auth-test
await_api

curl --fail --silent --show-error -b "$temp_dir/a.cookies" "$api_origin/v1/settings/project" >"$temp_dir/settings-after.json"
jq -e '.name == "Persisted QA project" and .retention_days == 90' "$temp_dir/settings-after.json" >/dev/null
curl --fail --silent --show-error -b "$temp_dir/a.cookies" "$api_origin/v1/environments" >"$temp_dir/environments-after.json"
jq -e '.data[] | select(.id == $id and .is_default == true)' --arg id "$environment_id" "$temp_dir/environments-after.json" >/dev/null
curl --fail --silent --show-error -b "$temp_dir/a.cookies" "$api_origin/v1/findings?status=resolved" >"$temp_dir/findings-after.json"
jq -e '.data[] | select(.id == $id and .resolved == true)' --arg id "$finding_id" "$temp_dir/findings-after.json" >/dev/null
curl --fail --silent --show-error -b "$temp_dir/a.cookies" "$api_origin/v1/notifications" >"$temp_dir/notifications-after.json"
jq -e '.data[] | select(.id == $id and (.read_at | type == "string"))' --arg id "$notification_id" "$temp_dir/notifications-after.json" >/dev/null

a_cookie=$(tr -d '\r' <"$temp_dir/a.headers" | sed -n 's/^Set-Cookie: \(paritylab_session=[^;]*\).*/\1/p' | head -1)
a_token=${a_cookie#paritylab_session=}
[ -n "$a_token" ]
logout_status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/logout.json" -D "$temp_dir/logout.headers" \
  -b "$temp_dir/a.cookies" -H "Origin: $web_origin" -X POST "$api_origin/v1/auth/logout")
expect_status 204 "$logout_status" "$temp_dir/logout.json"
tr -d '\r' <"$temp_dir/logout.headers" | grep -Eqi '^Set-Cookie: paritylab_session=.*; Max-Age=0;'
revoked_status=$(curl --silent --show-error --write-out '%{http_code}' --output "$temp_dir/revoked.json" \
  -H "Cookie: $a_cookie" "$api_origin/v1/session")
expect_status 401 "$revoked_status" "$temp_dir/revoked.json"

docker compose -p "$project_name" -f "$compose_file" logs api-auth-test >"$temp_dir/api.log"
! grep -F "$auth_password" "$temp_dir/api.log" >/dev/null
! grep -F 'sk_test_auth_resource_contract' "$temp_dir/api.log" >/dev/null
! grep -F "$a_token" "$temp_dir/api.log" >/dev/null
! grep -F "$b_token" "$temp_dir/api.log" >/dev/null

echo "auth security and restart contract passed"
