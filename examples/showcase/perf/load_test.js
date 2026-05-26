/**
 * k6 load test for the Showcase API.
 *
 * Scenarios:
 *   smoke   — 1 VU × 30 s  (sanity check, no errors expected)
 *   load    — ramp to 50 VU over 2 min, hold 3 min, ramp down
 *   stress  — ramp to 200 VU over 5 min (find the breaking point)
 *
 * Run:
 *   # smoke
 *   k6 run --env SCENARIO=smoke perf/load_test.js
 *
 *   # load (default)
 *   k6 run perf/load_test.js
 *
 *   # stress
 *   k6 run --env SCENARIO=stress perf/load_test.js
 *
 *   # against a remote host
 *   k6 run --env BASE_URL=http://staging.example.com perf/load_test.js
 */

import http from "k6/http";
import { check, sleep } from "k6";
import { Rate, Trend } from "k6/metrics";

// ── Config ────────────────────────────────────────────────────────────────────

const BASE_URL = __ENV.BASE_URL || "http://localhost:8080";
const SCENARIO = __ENV.SCENARIO || "load";

// A pre-issued JWT for a buyer account.
// Generate with: go run ./cmd/api --print-test-token  (or set via env)
const JWT = __ENV.JWT || "replace-with-a-valid-jwt";

// ── Custom metrics ────────────────────────────────────────────────────────────

const errorRate = new Rate("errors");
const orderLatency = new Trend("order_create_duration", true);

// ── Scenarios ─────────────────────────────────────────────────────────────────

const scenarios = {
  smoke: {
    executor: "constant-vus",
    vus: 1,
    duration: "30s",
  },
  load: {
    executor: "ramping-vus",
    startVUs: 0,
    stages: [
      { duration: "1m", target: 20 },
      { duration: "3m", target: 50 },
      { duration: "1m", target: 0 },
    ],
  },
  stress: {
    executor: "ramping-vus",
    startVUs: 0,
    stages: [
      { duration: "2m", target: 50 },
      { duration: "3m", target: 100 },
      { duration: "2m", target: 200 },
      { duration: "2m", target: 0 },
    ],
  },
};

export const options = {
  scenarios: { [SCENARIO]: scenarios[SCENARIO] },
  thresholds: {
    // 95th-percentile response time under 500 ms during load test
    http_req_duration: ["p(95)<500"],
    // Error rate below 1 %
    errors: ["rate<0.01"],
    // Order creation p95 under 1 s
    order_create_duration: ["p(95)<1000"],
  },
};

// ── Helpers ───────────────────────────────────────────────────────────────────

const authHeaders = {
  Authorization: `Bearer ${JWT}`,
  "Content-Type": "application/json",
};

function randomInt(min, max) {
  return Math.floor(Math.random() * (max - min + 1)) + min;
}

// ── Virtual user script ───────────────────────────────────────────────────────

export default function () {
  // 1. Health check (no auth)
  {
    const res = http.get(`${BASE_URL}/health`);
    check(res, { "health 200": (r) => r.status === 200 });
    errorRate.add(res.status !== 200);
  }

  sleep(0.2);

  // 2. List products (cached after first request)
  let productID = 1;
  {
    const res = http.get(`${BASE_URL}/api/v1/products?page=1&page_size=10`, {
      headers: authHeaders,
    });
    const ok = check(res, {
      "products 200": (r) => r.status === 200,
      "products has data": (r) => {
        try {
          const body = JSON.parse(r.body);
          return Array.isArray(body.data) && body.data.length > 0;
        } catch {
          return false;
        }
      },
    });
    errorRate.add(!ok);

    // Pick a random product from the response for the order below.
    try {
      const body = JSON.parse(res.body);
      if (body.data && body.data.length > 0) {
        productID = body.data[randomInt(0, body.data.length - 1)].id;
      }
    } catch (_) {}
  }

  sleep(0.3);

  // 3. Get single product
  {
    const res = http.get(`${BASE_URL}/api/v1/products/${productID}`, {
      headers: authHeaders,
    });
    check(res, { "product get 200": (r) => r.status === 200 });
    errorRate.add(res.status !== 200);
  }

  sleep(0.2);

  // 4. Create order (hot path — tracked separately)
  {
    const payload = JSON.stringify({
      items: [{ product_id: productID, qty: 1 }],
    });
    const start = Date.now();
    const res = http.post(`${BASE_URL}/api/v1/orders`, payload, {
      headers: authHeaders,
    });
    orderLatency.add(Date.now() - start);

    const ok = check(res, {
      "order created 201": (r) => r.status === 201,
    });
    // 409 (insufficient stock) is expected under load — don't count as error.
    errorRate.add(!ok && res.status !== 409);
  }

  sleep(randomInt(1, 3));
}

// ── Summary ───────────────────────────────────────────────────────────────────

export function handleSummary(data) {
  return {
    "perf/results.json": JSON.stringify(data, null, 2),
    stdout: textSummary(data),
  };
}

// Minimal inline text summary (avoids the k6/x/summary import).
function textSummary(data) {
  const m = data.metrics;
  const p95 = (metric) =>
    metric ? metric.values["p(95)"].toFixed(1) + " ms" : "n/a";
  return [
    "",
    "=== Showcase Load Test Summary ===",
    `  http_req_duration  p(95): ${p95(m.http_req_duration)}`,
    `  order_create       p(95): ${p95(m.order_create_duration)}`,
    `  error_rate:              ${((m.errors?.values.rate || 0) * 100).toFixed(2)} %`,
    `  total requests:          ${m.http_reqs?.values.count || 0}`,
    "",
  ].join("\n");
}
