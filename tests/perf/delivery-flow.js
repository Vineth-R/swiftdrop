import crypto from 'k6/crypto';
import encoding from 'k6/encoding';
import http from 'k6/http';
import { check, fail, sleep } from 'k6';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const CUSTOMER_EMAIL = __ENV.PERF_CUSTOMER_EMAIL;
const CUSTOMER_PASSWORD = __ENV.PERF_CUSTOMER_PASSWORD;
const DISPATCHER_EMAIL = __ENV.PERF_DISPATCHER_EMAIL;
const DISPATCHER_PASSWORD = __ENV.PERF_DISPATCHER_PASSWORD;
const COURIER_EMAIL = __ENV.PERF_COURIER_EMAIL;
const COURIER_PASSWORD = __ENV.PERF_COURIER_PASSWORD;
const PROOF_PNG = encoding.b64decode(
  'iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAYAAABytg0kAAAAH0lEQVR4nAASAO3/Av8AAP8AAAAAAAAAAAAAAAAAAwAfFwIBJnyuSgAAAABJRU5ErkJggg==',
  'std',
  's',
);

export const options = {
  scenarios: {
    delivery_flow: {
      executor: 'constant-vus',
      vus: Number(__ENV.PERF_VUS || 1),
      duration: __ENV.PERF_DURATION || '30s',
      gracefulStop: '10s',
    },
  },
  thresholds: {
    checks: ['rate>0.99'],
    http_req_failed: ['rate<0.05'],
  },
};

export function setup() {
  if (!CUSTOMER_EMAIL || !DISPATCHER_EMAIL || !COURIER_EMAIL) {
    fail('set PERF_CUSTOMER_*, PERF_DISPATCHER_* and PERF_COURIER_*');
  }
  const customer = login(CUSTOMER_EMAIL, CUSTOMER_PASSWORD);
  const dispatcher = login(DISPATCHER_EMAIL, DISPATCHER_PASSWORD);
  const courier = login(COURIER_EMAIL, COURIER_PASSWORD);
  const zones = http.get(`${BASE_URL}/api/v1/zones`, jsonHeaders());
  check(zones, { 'zones 200': (r) => r.status === 200 });
  const zoneList = zones.json('data');
  const origin = zoneList.find((zone) => zone.code === 'CMB');
  const destination = zoneList.find((zone) => zone.code === 'KDY');
  if (!origin || !destination) fail('CMB and KDY zones are required');
  const courierMe = http.get(`${BASE_URL}/api/v1/users/me`, jsonHeaders(authHeader(courier)));
  check(courierMe, { 'courier me 200': (r) => r.status === 200 });
  return {
    customer,
    dispatcher,
    courier,
    origin,
    destination,
    courierID: courierMe.json('data.id'),
  };
}

export default function (data) {
  const parcel = createParcel(data);
  const assigned = assignCourier(data, parcel.id);
  check(assigned, { 'assignment 200': (r) => r.status === 200 });
  const status = confirmPickup(data, parcel.id);
  check(status, { 'pickup 200': (r) => r.status === 200 });
  const propagated = waitForTracking(parcel.trackingNumber, 'IN_TRANSIT');
  check(propagated, { 'tracking propagated': (v) => v === true });
  sleep(1);
}

function createParcel(data) {
  const estimate = http.post(
    `${BASE_URL}/api/v1/pricing/estimate`,
    JSON.stringify({
      originZoneId: data.origin.id,
      destinationZoneId: data.destination.id,
      weightKg: 1,
      sizeCategory: 'SMALL',
    }),
    jsonHeaders(authHeader(data.customer)),
  );
  check(estimate, { 'estimate 200': (r) => r.status === 200 });
  const fee = estimate.json('data.total');
  const created = http.post(
    `${BASE_URL}/api/v1/parcels`,
    JSON.stringify({
      sender: contact('Perf Sender', data.origin),
      recipient: contact('Perf Recipient', data.destination),
      weightKg: 1,
      sizeCategory: 'SMALL',
      expectedFee: fee,
    }),
    jsonHeaders({ ...authHeader(data.customer), 'Idempotency-Key': uuidv4() }),
  );
  check(created, { 'parcel 201': (r) => r.status === 201 });
  return created.json('data');
}

function assignCourier(data, parcelID) {
  return http.put(
    `${BASE_URL}/api/v1/dispatch/parcels/${parcelID}/assignment`,
    JSON.stringify({ courierId: data.courierID, reason: 'k6 write-path baseline' }),
    jsonHeaders(authHeader(data.dispatcher)),
  );
}

function confirmPickup(data, parcelID) {
  return http.post(
    `${BASE_URL}/api/v1/courier/deliveries/${parcelID}/pickup`,
    { photo: http.file(PROOF_PNG, 'pickup.png', 'image/png') },
    { headers: authHeader(data.courier) },
  );
}

function waitForTracking(trackingNumber, status) {
  for (let attempt = 0; attempt < 50; attempt += 1) {
    const response = http.get(
      `${BASE_URL}/api/v1/tracking/${trackingNumber}`,
      jsonHeaders(),
    );
    if (response.status === 200 && response.json('data.status') === status) {
      return true;
    }
    sleep(0.1);
  }
  return false;
}

function login(email, password) {
  const response = http.post(
    `${BASE_URL}/api/v1/auth/login`,
    JSON.stringify({ email, password }),
    jsonHeaders({ 'X-Token-Mode': 'body' }),
  );
  check(response, { 'login 200': (r) => r.status === 200 });
  const token = response.json('data.accessToken');
  if (!token) fail(`login for ${email} returned no access token`);
  return token;
}

function authHeader(token) {
  return { Authorization: `Bearer ${token}` };
}

function jsonHeaders(extra = {}) {
  return { headers: { 'Content-Type': 'application/json', ...extra } };
}

function contact(name, zone) {
  return {
    name,
    phone: '+94771234567',
    address: '12 Perf Lane',
    city: zone.name,
    zoneId: zone.id,
  };
}

function uuidv4() {
  const bytes = Array.from(new Uint8Array(crypto.randomBytes(16)));
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = bytes.map((byte) => byte.toString(16).padStart(2, '0')).join('');
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}
