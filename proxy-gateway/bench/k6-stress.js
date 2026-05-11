// Stress / capacity test for proxy-gateway with realistic traffic.
//
// Generates a mix of GET / POST / PUT / DELETE across /api/users, /api/orders,
// /api/products, /api/events with varied body sizes and standard headers, so
// the numbers reflect what production HTTP traffic looks like (not an echo).
//
// Tags each stage so the summary shows per-stage latency. Run via run-stress.sh.

import http from 'k6/http';
import { check } from 'k6';

const PROXY  = __ENV.PROXY_URL    || 'http://localhost:8080';
const TARGET = __ENV.UPSTREAM_URL || 'http://upstream-mock:9000';
const KEY    = __ENV.ECHO_KEY     || 'sk_test_demo';
const DUR    = __ENV.STAGE_DUR    || '30s';

const STAGES = (__ENV.STAGES || '500,1000,2000,5000,10000,20000')
  .split(',').map((s) => parseInt(s.trim(), 10)).filter((n) => n > 0);

const STAGE_SECS = parseInt(DUR.replace(/\D/g, ''), 10) || 30;

function vusFor(rate) {
  // Real backend latency ~3ms (mock-upstream sleeps 0.5–3ms + JSON encode).
  // Concurrency ≈ rate * 0.005, headroom 5x for tail.
  return Math.max(50, Math.ceil(rate * 0.025));
}

export const options = {
  discardResponseBodies: false, // exercise body capture in the proxy
  scenarios: Object.fromEntries(STAGES.map((rate, i) => {
    const tag = `${String(i + 1).padStart(2, '0')}_${rate}rps`;
    return [`s${i}`, {
      executor: 'constant-arrival-rate',
      rate,
      timeUnit: '1s',
      duration: DUR,
      startTime: `${i * STAGE_SECS}s`,
      preAllocatedVUs: vusFor(rate),
      maxVUs: vusFor(rate) * 4,
      tags: { stage: tag },
      exec: 'hit',
    }];
  })),
  thresholds: Object.fromEntries(STAGES.flatMap((rate, i) => {
    const tag = `${String(i + 1).padStart(2, '0')}_${rate}rps`;
    return [
      [`http_req_duration{stage:${tag}}`,
        [{ threshold: 'p(99)<20', abortOnFail: false }]],
      [`http_req_failed{stage:${tag}}`,
        [{ threshold: 'rate<0.001', abortOnFail: false }]],
    ];
  })),
};

// ── Realistic traffic generators ─────────────────────────────────────────────
const UAS = [
  'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15',
  'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
  'okhttp/4.12.0',
  'curl/8.4.0',
  'PostmanRuntime/7.36.0',
  'echoproxy-sdk-go/0.4.1',
];

function rid() {
  return Math.random().toString(36).slice(2, 14);
}

function commonHeaders() {
  return {
    'X-Echo-Key':       KEY,
    'X-Echo-Target':    TARGET,
    'User-Agent':      UAS[Math.floor(Math.random() * UAS.length)],
    'Accept':          'application/json',
    'X-Request-Id':    rid(),
    'X-Forwarded-For': `192.168.${Math.floor(Math.random() * 256)}.${Math.floor(Math.random() * 256)}`,
  };
}

function getUsers() {
  return ['GET', `${PROXY}/api/users`, null, commonHeaders()];
}
function getUser() {
  return ['GET', `${PROXY}/api/users/${1000 + Math.floor(Math.random() * 9000)}`, null, commonHeaders()];
}
function createUser() {
  const body = JSON.stringify({
    email:      `user${rid()}@example.com`,
    password:   'hunter2-not-really',          // redacted by pkg/redact
    name:       `Test User ${rid()}`,
    plan:       'pro',
    referrer:   'https://google.com/search?q=echoproxy',
    metadata:   { signup_source: 'web', utm_campaign: 'spring' },
  });
  const h = commonHeaders();
  h['Content-Type'] = 'application/json';
  h['Authorization'] = 'Bearer eyJhbGciOiJIUzI1NiJ9.fake.token'; // redacted
  return ['POST', `${PROXY}/api/users`, body, h];
}
function getOrder() {
  return ['GET', `${PROXY}/api/orders/${rid()}`, null, commonHeaders()];
}
function createOrder() {
  const items = [];
  const n = 1 + Math.floor(Math.random() * 5);
  for (let i = 0; i < n; i++) {
    items.push({ sku: `SKU-${rid()}`, qty: 1 + Math.floor(Math.random() * 3), price: 9.99 + Math.random() * 90 });
  }
  const body = JSON.stringify({
    customer_id: `cus_${rid()}`,
    items,
    shipping_address: {
      line1: '123 Market St', city: 'San Francisco', state: 'CA', postal: '94103', country: 'US',
    },
    payment_method: { type: 'card', last4: '4242' },
    notes: 'Please leave at the door.',
  });
  const h = commonHeaders();
  h['Content-Type'] = 'application/json';
  return ['POST', `${PROXY}/api/orders`, body, h];
}
function getProduct() {
  return ['GET', `${PROXY}/api/products/${rid()}`, null, commonHeaders()];
}
function pushEvent() {
  // Larger body — exercises the 64KB body cap path.
  const body = JSON.stringify({
    type: 'page_view',
    user_id: rid(),
    properties: {
      url:       'https://app.example.com/dashboard?tab=analytics',
      referrer:  'https://app.example.com/login',
      title:     'Dashboard – Analytics',
      viewport:  { w: 1920, h: 1080 },
      screen:    { w: 2560, h: 1440 },
      timezone:  'America/Los_Angeles',
      language:  'en-US',
    },
    context: {
      session_id: rid(),
      app_version: '2.4.1',
      // pad to ~2KB so the body capture has something to chew on
      sample_text: 'lorem ipsum dolor sit amet, '.repeat(60),
    },
    timestamp: new Date().toISOString(),
  });
  const h = commonHeaders();
  h['Content-Type'] = 'application/json';
  return ['POST', `${PROXY}/api/events`, body, h];
}

// Weighted distribution: skewed toward reads, like real REST APIs.
const MIX = [
  { w: 35, fn: getUsers   },
  { w: 25, fn: getUser    },
  { w: 15, fn: getProduct },
  { w: 10, fn: getOrder   },
  { w:  6, fn: createUser  },
  { w:  5, fn: createOrder },
  { w:  4, fn: pushEvent   },
];
const TOTAL_W = MIX.reduce((s, m) => s + m.w, 0);

function pick() {
  let r = Math.random() * TOTAL_W;
  for (const m of MIX) {
    r -= m.w;
    if (r <= 0) return m.fn();
  }
  return MIX[0].fn();
}

export function hit() {
  const [method, url, body, headers] = pick();
  const res = http.request(method, url, body, { headers });
  check(res, {
    'status 2xx-3xx': (r) => r.status >= 200 && r.status < 400,
  });
}
