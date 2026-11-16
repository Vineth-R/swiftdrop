import assert from 'node:assert/strict';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import test from 'node:test';
import yaml from 'js-yaml';

const root = process.cwd();
const inventoryDirectory = 'contracts/routes';
const nginxPath = 'infra/nginx/default.conf';
const methods = ['get', 'post', 'put', 'patch', 'delete'];

// Every runtime inventory must be paired with the specifications its service
// owns. A service with no business contract (the webhook worker) is listed with
// an empty array so its operational probes stay explicit.
const serviceSpecs = {
  identity: ['identity.yaml', 'identity-internal.yaml'],
  parcel: ['parcel.yaml'],
  pricing: ['pricing.yaml'],
  merchant: ['merchant.yaml', 'merchant-internal.yaml'],
  'notification-worker': ['notification.yaml'],
  'ai-assistant': ['ai-assistant.yaml'],
  realtime: ['realtime.yaml'],
  'webhook-worker': [],
};

const serviceUpstreams = {
  identity: 'identity_upstream',
  parcel: 'parcel_upstream',
  pricing: 'pricing_upstream',
  merchant: 'merchant_upstream',
  'notification-worker': 'notification_worker_upstream',
  'ai-assistant': 'ai_assistant_upstream',
  realtime: 'realtime_upstream',
  'webhook-worker': null,
};

// These endpoints are registered by service entrypoints rather than the
// business routers. The gateway never maps them to their owning service.
const operationalPaths = new Set([
  '/health',
  '/ready',
  '/metrics',
  '/healthz',
  '/readyz',
]);

function canonicalPath(path) {
  const withoutPrefix = path.replace(/^\/api\/v1(?=\/|$)/, '');
  return (
    withoutPrefix
      .replace(/\{[^}]+\}/g, '{}')
      .replace(/:[^/]+/g, '{}')
      .replace(/\/+$/, '') || '/'
  );
}

function operationKey(method, path) {
  return `${method.toUpperCase()} ${canonicalPath(path)}`;
}

function loadSpecOperations(name) {
  const spec = yaml.load(
    readFileSync(join(root, 'contracts/openapi', name), 'utf8'),
  );
  const operations = [];
  for (const [path, item] of Object.entries(spec.paths ?? {})) {
    for (const method of methods) {
      if (item?.[method]) operations.push({ method, path });
    }
  }
  return operations;
}

function loadInventories() {
  const directory = join(root, inventoryDirectory);
  const inventories = new Map();
  for (const file of readdirSync(directory)
    .filter((entry) => entry.endsWith('.json'))
    .sort()) {
    const inventory = JSON.parse(readFileSync(join(directory, file), 'utf8'));
    assert.equal(
      inventory.service,
      file.replace(/\.json$/, ''),
      `${inventoryDirectory}/${file}: service does not match the filename`,
    );
    assert.ok(
      Array.isArray(inventory.routes),
      `${inventoryDirectory}/${file}: routes must be an array`,
    );
    inventories.set(inventory.service, inventory);
  }
  return inventories;
}

function specOperationsForService(service) {
  return (serviceSpecs[service] ?? []).flatMap(loadSpecOperations);
}

/**
 * Gin ↔ OpenAPI. Public and internal business routes must be documented; a
 * probe must be explicitly undocumented; operational probes are exempt.
 */
export function validateGinRoutes(inventory, documentedOperations) {
  const documented = new Set(
    documentedOperations.map(({ method, path }) => operationKey(method, path)),
  );
  const errors = [];
  for (const route of inventory.routes) {
    const key = operationKey(route.method, route.path);
    const isDocumented = documented.has(key);
    switch (route.classification) {
      case 'public':
      case 'internal':
        if (!route.documented) {
          errors.push(
            `${key}: ${route.classification} route is marked undocumented`,
          );
        }
        if (!isDocumented) {
          errors.push(
            `${key}: ${route.classification} route is not documented in OpenAPI`,
          );
        }
        break;
      case 'operational':
        if (route.documented) {
          errors.push(
            `${key}: operational route must not be in the business inventory`,
          );
        }
        break;
      case 'probe':
        if (route.documented) {
          errors.push(`${key}: probe route must be marked undocumented`);
        }
        if (isDocumented) {
          errors.push(`${key}: probe route must not appear in OpenAPI`);
        }
        break;
      default:
        errors.push(
          `${key}: unknown classification ${JSON.stringify(route.classification)}`,
        );
    }
    const expectedGateway =
      route.classification === 'public' || route.classification === 'probe';
    if (route.gateway !== expectedGateway) {
      errors.push(
        `${key}: gateway is ${route.gateway}, want ${expectedGateway} for ${route.classification}`,
      );
    }
  }
  return errors;
}

/**
 * OpenAPI ↔ Gin. Every documented operation except the operational health
 * endpoints must have a runtime registration. Path parameter names are
 * normalised, so /parcels/{id} matches /parcels/{parcelId}.
 */
export function validateDocumentedRoutes(inventory, documentedOperations) {
  const registered = new Set(
    inventory.routes.map((route) => operationKey(route.method, route.path)),
  );
  const errors = [];
  for (const { method, path } of documentedOperations) {
    if (operationalPaths.has(path)) continue;
    const key = operationKey(method, path);
    if (!registered.has(key)) {
      errors.push(`${key}: documented operation has no registered Gin route`);
    }
  }
  return errors;
}

function gatewayPaths(route) {
  if (route.path === '/ws') return ['/ws'];
  if (route.gateway) return [`/api/v1${route.path}`];
  if (route.classification === 'internal') {
    return [`/api/v1${route.path}`, route.path];
  }
  // Operational probes are mounted at the service root, but check the prefixed
  // form too so a future Nginx rule cannot expose them through /api/v1.
  return [route.path, `/api/v1${route.path}`];
}

/**
 * Gin ↔ Nginx. Gateway routes must reach the owning service; internal and
 * operational routes must never resolve to that service's upstream.
 */
export function validateGatewayMappings(
  service,
  inventory,
  locations,
  expectedUpstream,
) {
  const errors = [];
  for (const route of inventory.routes) {
    const key = operationKey(route.method, route.path);
    const paths = gatewayPaths(route);
    if (route.gateway) {
      if (!expectedUpstream) {
        errors.push(`${key}: ${service} has no configured gateway upstream`);
        continue;
      }
      const location = resolveLocation(paths[0], locations);
      if (!location) {
        errors.push(`${key}: no Nginx location maps ${paths[0]}`);
      } else if (!location.upstream) {
        errors.push(
          `${key}: Nginx location ${location.pattern} does not proxy ${paths[0]}`,
        );
      } else if (location.upstream !== expectedUpstream) {
        errors.push(
          `${key}: ${paths[0]} maps to ${location.upstream}, want ${expectedUpstream}`,
        );
      }
      continue;
    }

    for (const path of paths) {
      const location = resolveLocation(path, locations);
      if (location?.upstream === expectedUpstream) {
        errors.push(
          `${key}: ${route.classification} route is exposed through ${location.pattern} to ${expectedUpstream}`,
        );
      }
    }
  }
  return errors;
}

function prefixMatches(pattern, path) {
  if (path === pattern) return true;
  if (pattern.endsWith('/')) return path.startsWith(pattern);
  return path.startsWith(`${pattern}/`);
}

function redirectTarget(body) {
  const redirect = body.match(/\breturn\s+30[1278]\s+([^;\s]+)/);
  if (!redirect) return null;
  return redirect[1].replace(/\$is_args\$args$/, '').split('?')[0];
}

// Nginx adds a trailing slash with a relative 308 for collection routes, then
// the slash-suffixed location proxies to the service. Follow that redirect once
// so the effective upstream is the service, not the redirect itself.
function resolveLocation(publicPath, locations, depth = 0) {
  const location = effectiveLocation(publicPath, locations);
  if (!location || location.upstream || depth >= 2) return location;
  const target = redirectTarget(location.body);
  if (!target) return location;
  return resolveLocation(target, locations, depth + 1);
}

function matchesRegex(pattern, path) {
  try {
    return new RegExp(pattern).test(path);
  } catch {
    return false;
  }
}

function effectiveLocation(publicPath, locations) {
  const exact = locations.find(
    (location) => location.type === '=' && location.pattern === publicPath,
  );
  if (exact) return exact;

  const prefixes = locations
    .filter(
      (location) =>
        (location.type === '' || location.type === '^~') &&
        prefixMatches(location.pattern, publicPath),
    )
    .sort((left, right) => right.pattern.length - left.pattern.length);
  const prefix = prefixes[0];
  if (prefix?.type === '^~') return prefix;

  const regex = locations.find(
    (location) =>
      location.type === '~' && matchesRegex(location.pattern, publicPath),
  );
  return regex ?? prefix ?? null;
}

function skipQuoted(source, start) {
  const quote = source[start];
  for (let index = start + 1; index < source.length; index += 1) {
    if (source[index] === '\\') {
      index += 1;
      continue;
    }
    if (source[index] === quote) return index;
  }
  return source.length - 1;
}

function findBlockEnd(source, from) {
  let depth = 1;
  for (let index = from; index < source.length; index += 1) {
    const character = source[index];
    if (character === '#') {
      const newline = source.indexOf('\n', index);
      if (newline === -1) return source.length;
      index = newline;
      continue;
    }
    if (character === '"' || character === "'") {
      index = skipQuoted(source, index);
      continue;
    }
    if (character === '{') depth += 1;
    if (character === '}') {
      depth -= 1;
      if (depth === 0) return index;
    }
  }
  return source.length;
}

export function parseNginxLocations(source) {
  const locations = [];
  const pattern = /location\s+(?:(=|\^~|~)\s*)?([^\s{]+)\s*\{/g;
  let match;
  while ((match = pattern.exec(source))) {
    const start = match.index + match[0].length;
    const end = findBlockEnd(source, start);
    const body = source.slice(start, end);
    const proxy = body.match(/\bproxy_pass\s+https?:\/\/([^;:\s]+)/);
    locations.push({
      type: match[1] ?? '',
      pattern: match[2],
      upstream: proxy?.[1] ?? null,
      body,
    });
    pattern.lastIndex = end;
  }
  return locations;
}

function loadNginxLocations() {
  return parseNginxLocations(readFileSync(join(root, nginxPath), 'utf8'));
}

// ---------------------------------------------------------------------------
// Isolated failure-mode fixtures
// ---------------------------------------------------------------------------

const PARCEL_UPSTREAM = 'parcel_upstream';
const PRICING_UPSTREAM = 'pricing_upstream';

function publicRoute(path, method = 'GET') {
  return {
    method,
    path,
    classification: 'public',
    documented: true,
    gateway: true,
  };
}

function internalRoute(path, method = 'GET') {
  return {
    method,
    path,
    classification: 'internal',
    documented: true,
    gateway: false,
  };
}

function operationalRoute(path) {
  return {
    method: 'GET',
    path,
    classification: 'operational',
    documented: false,
    gateway: false,
    reason: 'operational probe',
  };
}

function probeRoute(path) {
  return {
    method: 'GET',
    path,
    classification: 'probe',
    documented: false,
    gateway: true,
    reason: 'access-test probe',
  };
}

function parcelInventory(...routes) {
  return { service: 'parcel', routes };
}

function location(pattern, upstream, type = '') {
  return { type, pattern, upstream, body: '' };
}

test('gate: a registered Gin route missing from OpenAPI fails', () => {
  const errors = validateGinRoutes(parcelInventory(publicRoute('/ghost')), []);
  assert.ok(
    errors.some((error) => error.includes('not documented in OpenAPI')),
    errors.join('\n'),
  );
});

test('gate: an OpenAPI operation without a Gin route fails', () => {
  const errors = validateDocumentedRoutes(parcelInventory(), [
    { method: 'GET', path: '/documented-only' },
  ]);
  assert.ok(
    errors.some((error) => error.includes('no registered Gin route')),
    errors.join('\n'),
  );
});

test('gate: a public route without a gateway location fails', () => {
  const errors = validateGatewayMappings(
    'parcel',
    parcelInventory(publicRoute('/parcels')),
    [],
    PARCEL_UPSTREAM,
  );
  assert.ok(
    errors.some((error) => error.includes('no Nginx location')),
    errors.join('\n'),
  );
});

test('gate: a public route mapped to the wrong upstream fails', () => {
  const errors = validateGatewayMappings(
    'parcel',
    parcelInventory(publicRoute('/parcels')),
    [location('/api/v1/parcels', PRICING_UPSTREAM, '^~')],
    PARCEL_UPSTREAM,
  );
  assert.ok(
    errors.some(
      (error) =>
        error.includes('maps to pricing_upstream') &&
        error.includes('want parcel_upstream'),
    ),
    errors.join('\n'),
  );
});

test('gate: an internal route exposed through the gateway fails', () => {
  const errors = validateGatewayMappings(
    'parcel',
    parcelInventory(internalRoute('/internal/parcels')),
    [location('/api/v1/internal', PARCEL_UPSTREAM, '^~')],
    PARCEL_UPSTREAM,
  );
  assert.ok(
    errors.some((error) => error.includes('is exposed through')),
    errors.join('\n'),
  );
});

test('gate: an explicit probe exception is classified and accepted', () => {
  const inventory = parcelInventory(
    probeRoute('/dispatch/probe'),
    operationalRoute('/healthz'),
  );
  assert.deepEqual(validateGinRoutes(inventory, []), []);
  assert.deepEqual(
    validateGatewayMappings(
      'parcel',
      inventory,
      [
        location('/api/v1/dispatch/', PARCEL_UPSTREAM),
        location('/healthz', null, '='),
      ],
      PARCEL_UPSTREAM,
    ),
    [],
  );
});

test('gate: different path parameter names normalise to the same route', () => {
  const inventory = parcelInventory(publicRoute('/parcels/{id}'));
  assert.deepEqual(
    validateGinRoutes(inventory, [
      { method: 'GET', path: '/parcels/{parcelId}' },
    ]),
    [],
  );
  assert.deepEqual(
    validateDocumentedRoutes(inventory, [
      { method: 'GET', path: '/parcels/{parcelId}' },
    ]),
    [],
  );
});

test('gate: an HTTP method mismatch fails in both directions', () => {
  const inventory = parcelInventory(publicRoute('/parcels', 'POST'));
  assert.ok(
    validateGinRoutes(inventory, [{ method: 'GET', path: '/parcels' }]).some(
      (error) => error.includes('not documented in OpenAPI'),
    ),
  );
  assert.ok(
    validateDocumentedRoutes(inventory, [
      { method: 'GET', path: '/parcels' },
    ]).some((error) => error.includes('no registered Gin route')),
  );
});

// ---------------------------------------------------------------------------
// Repository integration checks
// ---------------------------------------------------------------------------

test('route inventory files cover every HTTP service', () => {
  const inventories = loadInventories();
  assert.deepEqual(
    [...inventories.keys()].sort(),
    Object.keys(serviceSpecs).sort(),
  );
});

test('every Gin route matches its OpenAPI classification and documentation', () => {
  const inventories = loadInventories();
  const errors = [];
  for (const [service, inventory] of inventories) {
    errors.push(
      ...validateGinRoutes(inventory, specOperationsForService(service)).map(
        (error) => `${service}: ${error}`,
      ),
    );
  }
  assert.deepEqual(errors, []);
});

test('every documented OpenAPI operation has a registered Gin route', () => {
  const inventories = loadInventories();
  const errors = [];
  for (const [service, inventory] of inventories) {
    errors.push(
      ...validateDocumentedRoutes(
        inventory,
        specOperationsForService(service),
      ).map((error) => `${service}: ${error}`),
    );
  }
  assert.deepEqual(errors, []);
});

test('every gateway route maps to its owning service upstream', () => {
  const inventories = loadInventories();
  const locations = loadNginxLocations();
  const errors = [];
  for (const [service, inventory] of inventories) {
    errors.push(
      ...validateGatewayMappings(
        service,
        inventory,
        locations,
        serviceUpstreams[service],
      ).map((error) => `${service}: ${error}`),
    );
  }
  assert.deepEqual(errors, []);
});

test('the Nginx parser finds the expected upstreams', () => {
  const locations = loadNginxLocations();
  assert.ok(
    locations.some(
      (entry) =>
        entry.pattern === '/api/v1/admin/notification-logs' &&
        entry.upstream === 'notification_worker_upstream',
    ),
    'notification log location was not parsed',
  );
  assert.ok(
    locations.some(
      (entry) =>
        entry.pattern === '/ws' && entry.upstream === 'realtime_upstream',
    ),
    'WebSocket location was not parsed',
  );
  assert.ok(
    locations.some(
      (entry) =>
        entry.pattern === '^/api/v1/courier/deliveries/[^/]+/proof/?$' &&
        entry.upstream === 'parcel_upstream',
    ),
    'courier proof regex location was not parsed',
  );
});
