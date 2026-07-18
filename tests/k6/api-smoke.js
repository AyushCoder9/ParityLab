import http from "k6/http";
import { check } from "k6";

const apiURL = __ENV.PARITYLAB_API_URL || "http://127.0.0.1:8080";

export const options = {
  scenarios: {
    reads_and_runs: {
      executor: "constant-arrival-rate",
      rate: Number(__ENV.K6_REQUESTS_PER_SECOND || 20),
      timeUnit: "1s",
      duration: __ENV.K6_DURATION || "30s",
      preAllocatedVUs: 10,
      maxVUs: 50
    }
  },
  thresholds: {
    http_req_failed: ["rate<0.001"],
    http_req_duration: ["p(95)<500", "p(99)<1000"],
    checks: ["rate>0.999"]
  }
};

export default function () {
  const overview = http.get(`${apiURL}/v1/overview`, { tags: { name: "GET /v1/overview" } });
  check(overview, {
    "overview is 200": (response) => response.status === 200,
    "overview is sandbox": (response) => response.json("environment") === "sandbox",
    "overview has readiness": (response) => Number(response.json("readiness_score")) >= 0
  });

  const idempotencyKey = `k6-${__VU}-${__ITER}`;
  const run = http.post(
    `${apiURL}/v1/runs`,
    JSON.stringify({ scenario_id: "checkout-duplicate", fault: "duplicate" }),
    {
      headers: { "Content-Type": "application/json", "Idempotency-Key": idempotencyKey },
      tags: { name: "POST /v1/runs" }
    }
  );
  check(run, {
    "run accepted": (response) => response.status === 201,
    "run passed": (response) => response.json("status") === "passed",
    "run remains sandbox": (response) => response.json("environment") === "sandbox"
  });
}
