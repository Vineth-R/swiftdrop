import assert from 'node:assert/strict';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import test from 'node:test';
import yaml from 'js-yaml';

const contractDir = 'contracts/openapi';
const specs = readdirSync(contractDir)
  .filter((name) => name.endsWith('.yaml'))
  .sort();

const criticalPaths = {
  'identity.yaml': ['/auth/login', '/users/me', '/admin/users'],
  'identity-internal.yaml': ['/internal/users/{userId}/session-state'],
  'parcel.yaml': [
    '/parcels',
    '/tracking/{trackingNumber}',
    '/dispatch/unassigned',
    '/courier/deliveries',
    '/reports/daily',
  ],
  'pricing.yaml': ['/pricing/estimate', '/zones', '/admin/pricing/rules'],
  'merchant.yaml': [
    '/merchant/orders',
    '/merchants/me/webhook',
    '/admin/merchants',
  ],
  'merchant-internal.yaml': ['/internal/merchants/{merchantId}/webhook'],
  'notification.yaml': ['/admin/notification-logs'],
  'ai-assistant.yaml': ['/ai/chat'],
  'realtime.yaml': ['/ws', '/ready'],
};

function loadSpec(name) {
  const text = readFileSync(join(contractDir, name), 'utf8');
  return yaml.load(text);
}

function collectRefs(value, refs = []) {
  if (Array.isArray(value)) {
    for (const item of value) collectRefs(item, refs);
  } else if (value && typeof value === 'object') {
    for (const [key, item] of Object.entries(value)) {
      if (key === '$ref' && typeof item === 'string') refs.push(item);
      else collectRefs(item, refs);
    }
  }
  return refs;
}

function resolvePointer(spec, ref) {
  if (!ref.startsWith('#/')) return true;
  let current = spec;
  for (const part of ref.slice(2).split('/')) {
    const decoded = part.replaceAll('~1', '/').replaceAll('~0', '~');
    if (!current || typeof current !== 'object' || !(decoded in current))
      return false;
    current = current[decoded];
  }
  return true;
}

test('every OpenAPI specification parses and has the required top-level shape', () => {
  assert.ok(specs.length > 0, 'no OpenAPI files found');
  for (const name of specs) {
    const spec = loadSpec(name);
    assert.ok(spec, `${name} did not parse`);
    assert.match(
      String(spec.openapi),
      /^3\.0\./,
      `${name} must use OpenAPI 3.0.x`,
    );
    assert.ok(spec.info?.title, `${name} is missing info.title`);
    assert.ok(spec.info?.version, `${name} is missing info.version`);
    assert.ok(
      spec.paths && typeof spec.paths === 'object',
      `${name} is missing paths`,
    );
    assert.ok(Object.keys(spec.paths).length > 0, `${name} has no paths`);
    assert.ok(
      Array.isArray(spec.servers) && spec.servers.length > 0,
      `${name} is missing servers`,
    );
  }
});

test('every path and operation has a usable operation id and responses', () => {
  for (const name of specs) {
    const spec = loadSpec(name);
    const seen = new Set();
    for (const [path, item] of Object.entries(spec.paths)) {
      assert.ok(
        path.startsWith('/'),
        `${name}: path ${path} must start with /`,
      );
      assert.ok(
        item && typeof item === 'object',
        `${name}: ${path} is not an object`,
      );
      for (const [method, operation] of Object.entries(item)) {
        if (
          ![
            'get',
            'post',
            'put',
            'patch',
            'delete',
            'head',
            'options',
          ].includes(method)
        )
          continue;
        assert.ok(
          operation && typeof operation === 'object',
          `${name}: ${method} ${path} is invalid`,
        );
        assert.ok(
          operation.operationId,
          `${name}: ${method} ${path} has no operationId`,
        );
        assert.ok(
          !seen.has(operation.operationId),
          `${name}: duplicate operationId ${operation.operationId}`,
        );
        seen.add(operation.operationId);
        assert.ok(
          operation.responses && Object.keys(operation.responses).length > 0,
          `${name}: ${method} ${path} has no responses`,
        );
      }
    }
  }
});

test('all local component references resolve', () => {
  for (const name of specs) {
    const spec = loadSpec(name);
    for (const ref of collectRefs(spec)) {
      assert.ok(resolvePointer(spec, ref), `${name}: unresolved $ref ${ref}`);
    }
  }
});

test('critical public and internal routes remain documented', () => {
  for (const [name, paths] of Object.entries(criticalPaths)) {
    assert.ok(specs.includes(name), `${name} is missing`);
    const spec = loadSpec(name);
    for (const path of paths) {
      assert.ok(spec.paths[path], `${name} is missing ${path}`);
    }
  }
});

test('realtime documents the implemented WebSocket, not a future placeholder', () => {
  const spec = loadSpec('realtime.yaml');
  assert.ok(spec.paths['/ws']?.get, 'realtime /ws GET is missing');
  const text = JSON.stringify(spec);
  for (const message of [
    'subscribe',
    'unsubscribe',
    'ping',
    'subscribed',
    'parcel.status.updated',
    'parcel.assigned.updated',
    'error',
  ]) {
    assert.ok(
      text.includes(message),
      `realtime contract does not mention ${message}`,
    );
  }
  for (const check of ['consumer.status_changed', 'consumer.assigned']) {
    assert.ok(
      text.includes(check),
      `realtime readiness contract does not mention ${check}`,
    );
  }
});
