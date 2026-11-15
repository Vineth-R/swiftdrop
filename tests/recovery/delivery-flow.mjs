// Verifies that a real status event still flows through the relay and tracking
// projection after RabbitMQ has been restarted. Run after outage-recovery.sh.
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

async function request(path, { method = 'GET', token, body } = {}) {
  const response = await fetch(`${base}${path}`, {
    method,
    headers: {
      ...(body ? { 'Content-Type': 'application/json' } : {}),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
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
await request(`/api/v1/courier/deliveries/${parcel.id}/status`, {
  method: 'POST',
  token: courierToken,
  body: { status: 'PICKED_UP' },
});

const deadline = Date.now() + 20_000;
let tracking;
while (Date.now() < deadline) {
  tracking = (await request(`/api/v1/tracking/${parcel.trackingNumber}`)).data;
  if (tracking.status === 'PICKED_UP') break;
  await new Promise((resolve) => setTimeout(resolve, 250));
}
if (tracking?.status !== 'PICKED_UP') {
  throw new Error(
    `tracking did not reach PICKED_UP: ${JSON.stringify(tracking)}`,
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
