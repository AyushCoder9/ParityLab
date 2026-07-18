import crypto from "k6/crypto";
import exec from "k6/execution";
import http from "k6/http";
import { check } from "k6";

const apiURL = __ENV.PARITYLAB_API_URL || "http://127.0.0.1:8080";
const allowRemote = __ENV.PARITYLAB_ALLOW_REMOTE_LOAD === "1";
const loopback = /^https?:\/\/(localhost|127\.0\.0\.1|\[::1\])(?::\d+)?(?:\/|$)/.test(apiURL);

if (!loopback && !allowRemote) {
  throw new Error("Webhook burst is limited to loopback. Set PARITYLAB_ALLOW_REMOTE_LOAD=1 only for an authorized target.");
}

const deliveries = Number(__ENV.K6_BURST_EVENTS || 1_000);
if (!Number.isInteger(deliveries) || deliveries < 2 || deliveries % 2 !== 0) {
  throw new Error("K6_BURST_EVENTS must be an even integer of at least 2; every event is delivered twice.");
}

const endpointToken = __ENV.PARITYLAB_WEBHOOK_TOKEN || "demo";
const signingSecret = __ENV.PARITYLAB_WEBHOOK_SECRET || "whsec_paritylab_demo";

export const options = {
  scenarios: {
    duplicate_burst: {
      executor: "shared-iterations",
      vus: Math.min(Number(__ENV.K6_VUS || 50), deliveries / 2),
      iterations: deliveries / 2,
      maxDuration: __ENV.K6_MAX_DURATION || "2m"
    }
  },
  thresholds: {
    http_req_failed: ["rate<0.0001"],
    http_req_duration: ["p(95)<250", "p(99)<750"],
    checks: ["rate>0.9999"]
  }
};

function signedRequest(body) {
  const timestamp = Math.floor(Date.now() / 1000);
  const signature = crypto.hmac("sha256", signingSecret, `${timestamp}.${body}`, "hex");
  return {
    method: "POST",
    url: `${apiURL}/hooks/stripe/${endpointToken}`,
    body,
    params: {
      headers: {
        "Content-Type": "application/json",
        "Stripe-Signature": `t=${timestamp},v1=${signature}`
      },
      tags: { name: "POST /hooks/stripe/:token" }
    }
  };
}

export default function () {
  const eventID = `evt_k6_${exec.vu.idInTest}_${exec.scenario.iterationInTest}`;
  const body = JSON.stringify({
    id: eventID,
    object: "event",
    type: "payment_intent.succeeded",
    livemode: false,
    data: { object: { id: `pi_${eventID}` } }
  });
  const responses = http.batch([signedRequest(body), signedRequest(body)]);
  const duplicateFlags = responses.map((response) => response.json("duplicate"));
  check(responses, {
    "both deliveries accepted": ([first, second]) => first.status === 200 && second.status === 200,
    "exactly one delivery is duplicate": () => duplicateFlags.filter(Boolean).length === 1
  });
}
