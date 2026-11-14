import http from 'k6/http';
import { check, sleep } from 'k6';

const base = __ENV.BASE_URL || 'http://localhost:8080';
const tracking = __ENV.TRACKING_NUMBER || 'SDKUWNZTH2U';
const originZone =
  __ENV.ORIGIN_ZONE_ID || 'd273dcb3-8456-4574-a84a-ea568d5691f7';
const destinationZone =
  __ENV.DESTINATION_ZONE_ID || 'd0789d9e-c5a9-4477-8621-4b7cc9b39ef9';

export const options = {
  vus: Number(__ENV.VUS || 5),
  duration: __ENV.DURATION || '20s',
  summaryTrendStats: ['avg', 'p(50)', 'p(95)', 'p(99)', 'max'],
};

export default function () {
  const health = http.get(`${base}/healthz`);
  check(health, { 'gateway health 200': (r) => r.status === 200 });

  const track = http.get(`${base}/api/v1/tracking/${tracking}`);
  check(track, { 'public tracking 200': (r) => r.status === 200 });

  const estimate = http.post(
    `${base}/api/v1/pricing/estimate`,
    JSON.stringify({
      originZoneId: originZone,
      destinationZoneId: destinationZone,
      weightKg: 2,
      sizeCategory: 'SMALL',
    }),
    { headers: { 'Content-Type': 'application/json' } },
  );
  check(estimate, { 'pricing estimate 200': (r) => r.status === 200 });

  sleep(2.5);
}
