import http from 'k6/http';
import { check } from 'k6';

export const options = {
  scenarios: {
    main: {
      executor: 'constant-arrival-rate',
      rate: 5000,
      timeUnit: '1s',
      duration: '60s',
      preAllocatedVUs: 200,
      maxVUs: 500,
    },
  },
  thresholds: {
    'http_req_duration{expected_response:true}': ['p(99)<20', 'p(95)<10'],
    'http_req_failed': ['rate<0.001'],
    'proxy_dropped_events_total': ['count<100'],
  },
};

const PROXY = __ENV.PROXY_URL || 'http://localhost:8080';
const TARGET = __ENV.UPSTREAM_URL || 'http://upstream-mock:9000/echo';
const KEY = __ENV.ECHO_KEY || 'sk_test_demo';

export default function () {
  const res = http.post(PROXY + '/echo', JSON.stringify({ foo: 'bar' }), {
    headers: {
      'X-Echo-Key': KEY,
      'X-Echo-Target': TARGET,
      'Content-Type': 'application/json',
    },
  });
  check(res, { 'status 200': (r) => r.status === 200 });
}
