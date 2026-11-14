import http from 'k6/http';
import { check, fail, sleep } from 'k6';

const base = __ENV.BASE_URL || 'http://localhost:8080';
const email = __ENV.PERF_EMAIL;
const password = __ENV.PERF_PASSWORD;

export const options = {
  vus: Number(__ENV.VUS || 2),
  duration: __ENV.DURATION || '20s',
  summaryTrendStats: ['avg', 'p(50)', 'p(95)', 'p(99)', 'max'],
};

export function setup() {
  if (!email || !password) {
    fail('PERF_EMAIL and PERF_PASSWORD are required for the authentication scenario');
  }
}

export default function () {
  const response = http.post(
    `${base}/api/v1/auth/login`,
    JSON.stringify({ email, password }),
    { headers: { 'Content-Type': 'application/json' } },
  );
  check(response, {
    'login 200': (r) => r.status === 200,
    'session cookies set': (r) => String(r.headers['Set-Cookie'] || '').includes('sd_access='),
  });
  sleep(6);
}
