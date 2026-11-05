import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

import Ajv2020 from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';

const eventsDirectory = fileURLToPath(
  new URL('../../contracts/events/', import.meta.url),
);

const schemaFiles = {
  created: 'parcel.created.v1.schema.json',
  assigned: 'parcel.assigned.v1.schema.json',
  status: 'parcel.status_changed.v1.schema.json',
  notification: 'notification.requested.v1.schema.json',
};

const ajv = new Ajv2020({ allErrors: true, strict: true });
addFormats(ajv);

const schemas = Object.fromEntries(
  Object.entries(schemaFiles).map(([name, file]) => {
    const schema = JSON.parse(
      readFileSync(path.join(eventsDirectory, file), 'utf8'),
    );
    return [name, { file, schema, validate: ajv.compile(schema) }];
  }),
);

const created = {
  event_id: '0f8fad5b-d9cb-469f-a165-70867728950e',
  event_type: 'parcel.created.v1',
  event_version: 1,
  occurred_at: '2026-11-05T12:00:00Z',
  producer: 'parcel',
  data: {
    parcel_id: '6c8f8e56-5c6b-4d7c-9d9a-5f0f1f2b3c4d',
    tracking_number: 'SDK7M2Q9XP4',
    status: 'CREATED',
  },
};

const assigned = {
  event_id: '8c6a2f8e-3b9d-4a7e-9f1c-2d4e6f8a0b1c',
  event_type: 'parcel.assigned.v1',
  event_version: 1,
  occurred_at: '2026-11-05T12:05:00Z',
  producer: 'parcel',
  data: {
    parcel_id: '6c8f8e56-5c6b-4d7c-9d9a-5f0f1f2b3c4d',
    tracking_number: 'SDK7M2Q9XP4',
    courier_id: '2b3c4d5e-6f7a-4b8c-9d0e-1f2a3b4c5d6e',
    assigned_by: '3c4d5e6f-7a8b-4c9d-8e1f-2a3b4c5d6e7f',
  },
};

const status = {
  event_id: '7e5c3a1f-2b4d-4c6e-8f0a-1b3c5d7e9f2a',
  event_type: 'parcel.status_changed.v1',
  event_version: 1,
  occurred_at: '2026-11-05T12:10:00Z',
  producer: 'parcel',
  data: {
    parcel_id: '6c8f8e56-5c6b-4d7c-9d9a-5f0f1f2b3c4d',
    tracking_number: 'SDK7M2Q9XP4',
    previous_status: 'PICKED_UP',
    status: 'IN_TRANSIT',
  },
};

const notification = {
  event_id: '6c8f8e56-5c6b-4d7c-9d9a-5f0f1f2b3c4d',
  event_type: 'notification.requested.v1',
  event_version: 1,
  occurred_at: '2026-11-05T12:00:00Z',
  producer: 'parcel',
  data: {
    parcel_id: '6c8f8e56-5c6b-4d7c-9d9a-5f0f1f2b3c4d',
    recipient: 'customer@example.com',
    channel: 'email',
    template: 'Your parcel has shipped',
  },
};

function invalid(schema, payload) {
  return !schemas[schema].validate(payload);
}

test('all event schemas compile and their examples validate', () => {
  for (const { file, schema, validate } of Object.values(schemas)) {
    assert.equal(
      validate.errors,
      null,
      `${file} did not compile without validation errors`,
    );
    for (const [index, example] of (schema.examples ?? []).entries()) {
      assert.equal(
        validate(example),
        true,
        `${file} example ${index} is invalid: ${ajv.errorsText(validate.errors)}`,
      );
    }
  }
});

test('parcel.created accepts exactly one owner', () => {
  assert.equal(
    schemas.created.validate({
      ...created,
      data: {
        ...created.data,
        customer_user_id: '1e2f3a4b-5c6d-4e7f-8a9b-0c1d2e3f4a5b',
      },
    }),
    true,
  );
  assert.equal(
    schemas.created.validate({
      ...created,
      data: {
        ...created.data,
        merchant_id: '2b3c4d5e-6f7a-4b8c-9d0e-1f2a3b4c5d6e',
      },
    }),
    true,
  );
});

test('parcel.created rejects missing or duplicate owners', () => {
  assert.equal(invalid('created', created), true);
  assert.equal(
    invalid('created', {
      ...created,
      data: {
        ...created.data,
        customer_user_id: '1e2f3a4b-5c6d-4e7f-8a9b-0c1d2e3f4a5b',
        merchant_id: '2b3c4d5e-6f7a-4b8c-9d0e-1f2a3b4c5d6e',
      },
    }),
    true,
  );
});

test('parcel events reject foreign producers', () => {
  for (const [name, payload] of Object.entries({ created, assigned, status })) {
    assert.equal(invalid(name, { ...payload, producer: 'not-parcel' }), true);
  }
});

test('parcel events reject invalid event types and versions', () => {
  for (const [name, payload] of Object.entries({ created, assigned, status })) {
    assert.equal(
      invalid(name, { ...payload, event_type: 'parcel.unknown.v1' }),
      true,
    );
    assert.equal(invalid(name, { ...payload, event_version: 2 }), true);
  }
});

test('parcel events reject unexpected payload fields', () => {
  for (const [name, payload] of Object.entries({ created, assigned, status })) {
    assert.equal(
      invalid(name, {
        ...payload,
        data: { ...payload.data, email: 'customer@example.com' },
      }),
      true,
    );
  }
});

test('parcel.created rejects invalid initial statuses', () => {
  assert.equal(
    invalid('created', {
      ...created,
      data: { ...created.data, status: 'PICKED_UP' },
    }),
    true,
  );
});

test('parcel.status_changed rejects invalid statuses', () => {
  assert.equal(
    invalid('status', {
      ...status,
      data: { ...status.data, status: 'PENDING' },
    }),
    true,
  );
});

test('parcel.status_changed requires failure_reason for FAILED', () => {
  assert.equal(
    invalid('status', {
      ...status,
      data: { ...status.data, status: 'FAILED' },
    }),
    true,
  );
  assert.equal(
    invalid('status', {
      ...status,
      data: {
        ...status.data,
        status: 'FAILED',
        failure_reason: 'bad',
      },
    }),
    true,
  );
  assert.equal(
    schemas.status.validate({
      ...status,
      data: {
        ...status.data,
        status: 'FAILED',
        failure_reason: 'Recipient refused delivery',
      },
    }),
    true,
  );
});

test('the legacy notification schema remains compatible', () => {
  assert.equal(schemas.notification.validate(notification), true);
  assert.equal(
    invalid('notification', {
      ...notification,
      data: { ...notification.data, email: 'customer@example.com' },
    }),
    true,
  );
});
