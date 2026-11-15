import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import test from 'node:test';
import yaml from 'js-yaml';

const root = process.cwd();
const contractDir = 'contracts/openapi';
const matrixPath = 'tests/access/matrix_test.go';
const nginxPath = 'infra/nginx/default.conf';

const publicSpecs = {
  'identity.yaml': [
    '/api/v1/auth',
    '/api/v1/users',
    '/api/v1/admin/users',
    '/api/v1/couriers',
  ],
  'pricing.yaml': [
    '/api/v1/pricing',
    '/api/v1/zones',
    '/api/v1/admin/pricing',
    '/api/v1/admin/zones',
  ],
  'parcel.yaml': [
    '/api/v1/parcels',
    '/api/v1/tracking',
    '/api/v1/dispatch',
    '/api/v1/admin/parcels',
    '/api/v1/courier',
    '/api/v1/reports',
  ],
  'merchant.yaml': [
    '/api/v1/merchants',
    '/api/v1/merchant',
    '/api/v1/admin/merchants',
  ],
  'notification.yaml': ['/api/v1/admin/notification-logs'],
  'ai-assistant.yaml': ['/api/v1/ai/chat'],
  'realtime.yaml': ['/ws'],
};

const methods = ['get', 'post', 'put', 'patch', 'delete'];

// Public operations do not require a session. Everything else in the public
// specs must be present in the access matrix unless it is an API-key-only route
// or an internal probe that Nginx never exposes.
const publicOperations = new Set(
  [
    'POST /auth/register',
    'POST /auth/login',
    'POST /auth/refresh',
    'POST /auth/verify-email',
    'POST /auth/resend-verification',
    'GET /auth/google/login',
    'GET /auth/google/callback',
    'POST /auth/forgot-password',
    'POST /auth/reset-password',
    'POST /merchants/register',
    'POST /pricing/estimate',
    'GET /zones',
    'GET /tracking/{trackingNumber}',
    'POST /ai/chat',
    'GET /ws',
  ].map((key) => {
    const [method, ...path] = key.split(' ');
    return operationKey(method, path.join(' '));
  }),
);

const accessExceptions = new Set([
  'POST /merchant/orders', // API 8.16 is API-key-only; the browser never holds a key.
]);

const internalOperation = (path) =>
  path.startsWith('/internal/') || path === '/health' || path === '/ready';

function loadSpec(name) {
  return yaml.load(readFileSync(join(root, contractDir, name), 'utf8'));
}

function normalizePath(path) {
  const withoutPrefix = path.replace(/^\/api\/v1(?=\/|$)/, '');
  return (
    withoutPrefix
      .replace(/\{[^}]+\}/g, '{}')
      .replace(/:[^/]+/g, '{}')
      .replace(
        /\/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi,
        '/{}',
      )
      .replace(/\/SD[KN][A-Z0-9]{8}(?=\/|$)/g, '/{}')
      .replace(
        /^\/admin\/pricing\/surcharges\/[^/]+$/,
        '/admin/pricing/surcharges/{}',
      )
      .replace(/\/+$/, '') || '/'
  );
}

function operationKey(method, path) {
  return `${method.toUpperCase()} ${normalizePath(path)}`;
}

function specOperations(spec) {
  const operations = [];
  for (const [path, item] of Object.entries(spec.paths ?? {})) {
    for (const method of methods) {
      if (item?.[method]) operations.push({ method, path });
    }
  }
  return operations;
}

function matrixOperations() {
  const source = readFileSync(join(root, matrixPath), 'utf8');
  const entries = [];
  const entryPattern =
    /\{\s*Name:\s*"[^"]+",\s*Method:\s*http\.Method([A-Za-z]+),\s*Path:\s*([^,]+),/g;
  for (const match of source.matchAll(entryPattern)) {
    const method = match[1].toLowerCase();
    const tokens = match[2].match(/"(?:[^"\\]|\\.)*"|\w+/g) ?? [];
    const path = tokens
      .map((token) => (token.startsWith('"') ? JSON.parse(token) : '{}'))
      .join('');
    entries.push({ method, path });
  }
  return entries;
}

function nginxLocations() {
  const source = readFileSync(join(root, nginxPath), 'utf8');
  return [
    ...source.matchAll(/^\s*location\s+(?:=\s+|\^~\s+|~\s+)?([^\s{]+)/gm),
  ].map((match) => match[1]);
}

function hasGatewayMapping(path, prefixes, locations) {
  const publicPath = path === '/ws' ? path : `/api/v1${path}`;
  const prefix = prefixes.find(
    (candidate) =>
      publicPath === candidate || publicPath.startsWith(`${candidate}/`),
  );
  if (!prefix) return false;
  return locations.some(
    (location) =>
      location === prefix ||
      location.startsWith(`${prefix}/`) ||
      location.startsWith(`${prefix}[`),
  );
}

test('public OpenAPI operations are either session-protected in the access matrix or explicitly public', () => {
  const matrix = new Set(
    matrixOperations().map(({ method, path }) => operationKey(method, path)),
  );

  for (const name of Object.keys(publicSpecs)) {
    const spec = loadSpec(name);
    for (const operation of specOperations(spec)) {
      if (internalOperation(operation.path)) continue;
      const key = operationKey(operation.method, operation.path);
      if (publicOperations.has(key) || accessExceptions.has(key)) continue;
      assert.ok(
        matrix.has(key),
        `${name}: ${operation.method.toUpperCase()} ${operation.path} is neither public nor in ${matrixPath}`,
      );
    }
  }
});

test('every access-matrix route is documented in a public OpenAPI specification', () => {
  const documented = new Set();
  for (const name of Object.keys(publicSpecs)) {
    const spec = loadSpec(name);
    for (const operation of specOperations(spec)) {
      documented.add(operationKey(operation.method, operation.path));
    }
  }

  for (const { method, path } of matrixOperations()) {
    const key = operationKey(method, path);
    assert.ok(
      documented.has(key),
      `${matrixPath}: ${method.toUpperCase()} ${path} is not documented in a public OpenAPI specification`,
    );
  }
});

test('every public OpenAPI path has a gateway location', () => {
  const locations = nginxLocations();

  for (const [name, prefixes] of Object.entries(publicSpecs)) {
    const spec = loadSpec(name);
    for (const operation of specOperations(spec)) {
      if (internalOperation(operation.path)) continue;
      assert.ok(
        hasGatewayMapping(operation.path, prefixes, locations),
        `${name}: ${operation.method.toUpperCase()} ${operation.path} has no ${nginxPath} location`,
      );
    }
  }
});
