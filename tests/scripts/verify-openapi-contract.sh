#!/bin/sh
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
contract="$repo_dir/packages/contracts/openapi.yaml"

require() {
  pattern=$1
  description=$2
  if ! rg --quiet --multiline "$pattern" "$contract"; then
    echo "OpenAPI contract missing: $description" >&2
    exit 1
  fi
}

require '^  /v1/connections/stripe/validate:' 'Stripe Sandbox validation endpoint'
require '^      operationId: validateStripeConnection$' 'Stripe validation operation ID'
require 'secret_key:\n                  type: string\n                  writeOnly: true' 'write-only Stripe secret'
require '^    StripeConnection:\n      type: object\n      additionalProperties: false\n      required: \[id, stripe_account_id, sandbox_name, status, created_at\]' 'sanitized Stripe connection response'
require '^  /v1/stripe/payment-intents:' 'Stripe PaymentIntent verification endpoint'
require '^      operationId: createStripePaymentIntentRun$' 'Stripe PaymentIntent operation ID'
require 'amount_minor: \{ type: integer, minimum: 1, maximum: 99999999 \}' 'integer minor-unit constraint'
require "currency: \{ type: string, pattern: '\^\[a-z\]\{3\}" 'lowercase ISO currency constraint'
require '^  /v1/runs/\{id\}/events:' 'run event stream endpoint'
require '^      operationId: getRunEvents$' 'run event stream operation ID'
require 'name: Last-Event-ID\n          in: header' 'resumable SSE cursor header'
require 'text/event-stream:' 'SSE response media type'

echo "OpenAPI product contract validation passed"
