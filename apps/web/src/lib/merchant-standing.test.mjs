import assert from 'node:assert/strict';
import { test } from 'node:test';

const { merchantStandingFromIssues } = await import('./merchant-standing.ts');

test('reads the status, the reason and the support address', () => {
  assert.deepEqual(
    merchantStandingFromIssues([
      { field: 'status', issue: 'REJECTED' },
      { field: 'reason', issue: '  The registration number is not valid.  ' },
      { field: 'supportEmail', issue: 'support@swiftdrop.example' },
    ]),
    {
      status: 'REJECTED',
      reason: 'The registration number is not valid.',
      supportEmail: 'support@swiftdrop.example',
    },
  );
});

test('a status alone is enough, and the other parts stay empty', () => {
  assert.deepEqual(
    merchantStandingFromIssues([{ field: 'status', issue: 'PENDING' }]),
    {
      status: 'PENDING',
      reason: undefined,
      supportEmail: undefined,
    },
  );
});

test('no details, or a status that is not one of the three, is the plain refusal', () => {
  assert.equal(merchantStandingFromIssues([]), undefined);
  assert.equal(
    merchantStandingFromIssues([{ field: 'status', issue: 'APPROVED' }]),
    undefined,
  );
  assert.equal(
    merchantStandingFromIssues([
      { field: 'reason', issue: 'No status given.' },
    ]),
    undefined,
  );
});

test('a support address that is not a plain address never reaches a link', () => {
  for (const issue of [
    'javascript:alert(1)',
    'a@b.c?cc=x@y.z extra',
    'two words@example.com',
    '',
  ]) {
    const standing = merchantStandingFromIssues([
      { field: 'status', issue: 'SUSPENDED' },
      { field: 'supportEmail', issue },
    ]);
    assert.equal(standing?.supportEmail, undefined, issue);
  }
});
