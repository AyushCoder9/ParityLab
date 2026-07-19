import { createServer } from "node:http";

const port = Number.parseInt(process.env.PORT ?? "12111", 10);

function send(response, status, value) {
  response.writeHead(status, { "content-type": "application/json" });
  response.end(JSON.stringify(value));
}

async function body(request) {
  const chunks = [];
  for await (const chunk of request) chunks.push(chunk);
  return Buffer.concat(chunks).toString("utf8");
}

createServer(async (request, response) => {
  process.stdout.write(`${request.method} ${request.url}\n`);
  if (request.method === "GET" && request.url === "/healthz") {
    return send(response, 200, { status: "ok" });
  }

  const authorization = request.headers.authorization ?? "";
  if (!/^Bearer (?:sk|rk)_test_/.test(authorization)) {
    return send(response, 401, { error: { type: "invalid_request_error", code: "api_key_invalid", message: "Invalid test API key" } });
  }

  if (request.method === "GET" && request.url === "/v1/account") {
    return send(response, 200, {
      id: "acct_mock_sandbox",
      object: "account",
      livemode: false,
      charges_enabled: true,
      payouts_enabled: false,
      details_submitted: true,
    });
  }

  if (request.method === "POST" && request.url === "/v1/payment_intents") {
    const form = new URLSearchParams(await body(request));
    const amount = Number.parseInt(form.get("amount") ?? "", 10);
    const currency = form.get("currency") ?? "";
    const correlationID = form.get("metadata[paritylab_correlation_id]") ?? "";
    const scenarioID = form.get("metadata[paritylab_scenario_id]") ?? "";
    if (!Number.isSafeInteger(amount) || amount <= 0 || !/^[a-z]{3}$/.test(currency) || !/^plcorr_[a-f0-9]{24,64}$/.test(correlationID) || scenarioID !== "checkout-duplicate") {
      process.stderr.write(`PaymentIntent contract violation: ${JSON.stringify(Object.fromEntries(form))}\n`);
      return send(response, 422, { error: { type: "invalid_request_error", code: "mock_contract_violation", message: "PaymentIntent request did not match the ParityLab contract" } });
    }
    return send(response, 200, {
      id: `pi_mock_${correlationID.slice(7, 19)}`,
      object: "payment_intent",
      amount,
      currency,
      status: "succeeded",
      livemode: false,
      metadata: { paritylab_correlation_id: correlationID, paritylab_scenario_id: scenarioID },
    });
  }

  return send(response, 404, { error: { type: "invalid_request_error", code: "mock_route_not_found", message: `${request.method} ${request.url}` } });
}).listen(port, "0.0.0.0", () => {
  process.stdout.write(`Stripe contract mock listening on ${port}\n`);
});
