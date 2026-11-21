// Verifies that a real pickup event still flows through the relay and tracking
// projection after RabbitMQ has been restarted. Run after outage-recovery.sh.
import { Buffer } from 'node:buffer';

const PROOF_PNG = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAYAAABytg0kAAAAH0lEQVR4nAASAO3/Av8AAP8AAAAAAAAAAAAAAAAAAwAfFwIBJnyuSgAAAABJRU5ErkJggg==',
  'base64',
);
const base = process.env.REAL_STACK_BASE_URL || 'http://localhost:8080';
const customer = {
  email: process.env.REAL_STACK_CUSTOMER_EMAIL,
  password: process.env.REAL_STACK_CUSTOMER_PASSWORD,
};
const dispatcher = {
  email: process.env.REAL_STACK_DISPATCHER_EMAIL,
  password: process.env.REAL_STACK_DISPATCHER_PASSWORD,
};
const courier = {
  email: process.env.REAL_STACK_COURIER_1_EMAIL,
  password: process.env.REAL_STACK_COURIER_1_PASSWORD,
};
if (!customer.email || !dispatcher.email || !courier.email) {
  throw new Error('real-stack seeded account environment is required');
}

async function request(path, { method = 'GET', token, body, headers = {} } = {}) {
  const response = await fetch(`${base}${path}`, {
    method,
    headers: {
      ...(body ? { 'Content-Type': 'application/json' } : {}),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...headers,
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await response.text();
  if (!response.ok) {
    throw new Error(`${method} ${path} returned ${response.status}: ${text}`);
  }
  return text ? JSON.parse(text) : null;
}

async function login(account) {
  const response = await request('/api/v1/auth/login', {
    method: 'POST',
    headers: { 'X-Token-Mode': 'body' },
    body: { email: account.email, password: account.password },
  });
  const token = response?.data?.accessToken;
  if (!token) throw new Error(`login for ${account.email} returned no token`);
  return token;
}

const customerToken = await login(customer);
const dispatcherToken = await login(dispatcher);
const courierToken = await login(courier);
const zones = (await request('/api/v1/zones')).data;
const origin = zones.find((zone) => zone.code === 'CMB');
const destination = zones.find((zone) => zone.code === 'KDY');
if (!origin || !destination) throw new Error('CMB and KDY zones are required');

const estimate = await request('/api/v1/pricing/estimate', {
  method: 'POST',
  token: customerToken,
  body: {
    originZoneId: origin.id,
    destinationZoneId: destination.id,
    weightKg: 1,
    sizeCategory: 'SMALL',
  },
});
const created = await request('/api/v1/parcels', {
  method: 'POST',
  token: customerToken,
  body: {
    sender: contact('Recovery Sender', origin),
    recipient: contact('Recovery Recipient', destination),
    weightKg: 1,
    sizeCategory: 'SMALL',
    expectedFee: estimate.data.total,
  },
});
const parcel = created.data;
const me = await request('/api/v1/users/me', { token: courierToken });
await request(`/api/v1/dispatch/parcels/${parcel.id}/assignment`, {
  method: 'PUT',
  token: dispatcherToken,
  body: { courierId: me.data.id, reason: 'Outage recovery test' },
});
// One pickup action records PICKED_UP and IN_TRANSIT; tracking settles on
// IN_TRANSIT after the relay publishes both events.
const photo = new FormData();
photo.append(
  'photo',
  new Blob([PROOF_PNG], { type: 'image/png' }),
  'pickup.png',
);
const pickupResponse = await fetch(
  `${base}/api/v1/courier/deliveries/${parcel.id}/pickup`,
  {
    method: 'POST',
    headers: { Authorization: `Bearer ${courierToken}` },
    body: photo,
  },
);
const pickupText = await pickupResponse.text();
if (!pickupResponse.ok) {
  throw new Error(
    `POST /api/v1/courier/deliveries/${parcel.id}/pickup returned ${pickupResponse.status}: ${pickupText}`,
  );
}

const deadline = Date.now() + 20_000;
let tracking;
while (Date.now() < deadline) {
  tracking = (await request(`/api/v1/tracking/${parcel.trackingNumber}`)).data;
  if (tracking.status === 'IN_TRANSIT') break;
  await new Promise((resolve) => setTimeout(resolve, 250));
}
if (tracking?.status !== 'IN_TRANSIT') {
  throw new Error(
    `tracking did not reach IN_TRANSIT: ${JSON.stringify(tracking)}`,
  );
}
console.log(
  JSON.stringify({
    parcelId: parcel.id,
    trackingNumber: parcel.trackingNumber,
    status: tracking.status,
  }),
);

function contact(name, zone) {
  return {
    name,
    phone: '+94771234567',
    address: '12 Temple Road',
    city: zone.name,
    zoneId: zone.id,
  };
}
