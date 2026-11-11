import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const specification = readFileSync(
  fileURLToPath(
    new URL('../../contracts/openapi/merchant.yaml', import.meta.url),
  ),
  'utf8',
);

const start = specification.indexOf('  /merchants/register:');
const end = specification.indexOf('  /merchants/me:', start);
assert.notEqual(start, -1, 'merchant registration path is missing');
assert.notEqual(end, -1, 'merchant profile path is missing');
const registration = specification.slice(start, end);

test('merchant registration requires a business registration number', () => {
  assert.match(
    registration,
    /required:\s*\[\s*name,\s*registrationNumber,/s,
    'business.registrationNumber is not in the required list',
  );
  assert.match(
    registration,
    /registrationNumber:\s*\n\s*type: string\s*\n\s*minLength: 3\s*\n\s*maxLength: 50/,
    'registrationNumber schema must be a 3-50 character string',
  );
});
