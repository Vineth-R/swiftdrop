import assert from 'node:assert/strict';
import { test } from 'node:test';

// Teaches Node to load the TypeScript modules under test.
import './api-test-support.mjs';

const {
  serverFieldMessage,
  validateAddress,
  validateBusinessName,
  validateIfFilled,
  validatePhone,
  validateRegistrationNumber,
} = await import('./validation.ts');

test('a registration number follows the rules the merchant API enforces', () => {
  for (const good of [
    'PV 123456',
    'pv   123456',
    'GV-2026/0457',
    'HE.98',
    '123',
  ]) {
    assert.equal(validateRegistrationNumber(good), undefined, good);
  }
  for (const bad of [
    '',
    '  ',
    'PV',
    'ABCDEF', // no digit
    'PV 123456 #',
    '1'.repeat(51),
    'PV_123456',
  ]) {
    assert.notEqual(validateRegistrationNumber(bad), undefined, bad);
  }
});

test('names and addresses are counted in characters, not bytes', () => {
  assert.equal(validateBusinessName('ශ්‍රී'), undefined);
  assert.notEqual(validateBusinessName('A'), undefined);
  assert.notEqual(validateBusinessName('a'.repeat(151)), undefined);
  assert.equal(validateAddress('88 Galle Road'), undefined);
  assert.notEqual(validateAddress('Road'), undefined);
  assert.notEqual(validateAddress('a'.repeat(256)), undefined);
});

test('an optional field is checked only when something was typed', () => {
  assert.equal(validateIfFilled('', validatePhone), undefined);
  assert.equal(validateIfFilled('   ', validatePhone), undefined);
  assert.notEqual(validateIfFilled('123', validatePhone), undefined);
  assert.equal(validateIfFilled('71 234 5678', validatePhone), undefined);
});

test('every business field the API can reject has our own sentence', () => {
  for (const field of [
    'business.name',
    'business.registrationNumber',
    'business.contactEmail',
    'business.contactPhone',
    'business.businessAddress',
    'business.pickupAddress',
    'business.pickupZoneId',
  ]) {
    assert.ok(serverFieldMessage(field), field);
  }
});
