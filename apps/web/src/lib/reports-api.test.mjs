import assert from 'node:assert/strict';
import { afterEach, test } from 'node:test';

import {
  fail,
  failNetwork,
  ok,
  restoreFetch,
  stubFetch,
} from './api-test-support.mjs';

const {
  defaultRange,
  formatReportDate,
  getDailyReport,
  getOutcomesReport,
  rangeErrorsFromIssues,
  shiftDays,
  validateRange,
} = await import('./reports-api.ts');

const range = { from: '2026-11-01', to: '2026-11-05' };

const daily = [
  { date: '2026-11-01', created: 42, delivered: 35, failed: 3 },
  { date: '2026-11-02', created: 51, delivered: 44, failed: 2 },
];

const outcomes = {
  from: '2026-10-04',
  to: '2026-11-03',
  totals: { created: 410, delivered: 352, failed: 21, inProgress: 37 },
  successRate: 94.37,
  failureRate: 5.63,
  topFailureReasons: [
    { reason: 'Recipient not available', count: 9 },
    { reason: 'Incorrect address', count: 6 },
  ],
};

afterEach(restoreFetch);

test('defaultRange returns the last 30 inclusive UTC dates', () => {
  assert.deepEqual(defaultRange(30, new Date('2026-11-05T12:00:00Z')), {
    from: '2026-10-07',
    to: '2026-11-05',
  });
});

test('defaultRange reads "today" in UTC, not local time', () => {
  assert.deepEqual(defaultRange(1, new Date('2026-11-05T23:30:00-05:00')), {
    from: '2026-11-06',
    to: '2026-11-06',
  });
});

test('defaultRange count includes both ends', () => {
  const { from, to } = defaultRange(30, new Date('2026-11-05T00:00:00Z'));
  const days =
    (Date.parse(`${to}T00:00:00Z`) - Date.parse(`${from}T00:00:00Z`)) /
      86_400_000 +
    1;
  assert.equal(days, 30);
});

for (const [date, days, want] of [
  ['2026-11-01', 1, '2026-11-02'],
  ['2026-11-01', -1, '2026-10-31'],
  ['2026-01-31', 1, '2026-02-01'],
  ['2026-03-01', -1, '2026-02-28'],
  ['2026-12-31', 1, '2027-01-01'],
  ['2024-02-28', 1, '2024-02-29'],
  ['2024-02-29', 1, '2024-03-01'],
  ['2026-03-28', 1, '2026-03-29'],
  ['2026-03-29', 1, '2026-03-30'],
]) {
  test(`shiftDays(${date}, ${days}) is ${want}`, () => {
    assert.equal(shiftDays(date, days), want);
  });
}

test('validateRange accepts a valid range', () => {
  assert.deepEqual(validateRange(range), {});
});

test('validateRange accepts a single day', () => {
  assert.deepEqual(validateRange({ from: '2026-11-05', to: '2026-11-05' }), {});
});

test('validateRange accepts exactly 90 inclusive days', () => {
  assert.deepEqual(validateRange({ from: '2026-01-01', to: '2026-03-31' }), {});
});

test('validateRange rejects 91 inclusive days', () => {
  assert.deepEqual(validateRange({ from: '2026-01-01', to: '2026-04-01' }), {
    to: 'Choose a range of 90 days or fewer.',
  });
});

test('validateRange rejects a backwards range', () => {
  assert.deepEqual(validateRange({ from: '2026-11-05', to: '2026-11-01' }), {
    to: 'The end date must be on or after the start date.',
  });
});

test('validateRange rejects empty dates', () => {
  assert.deepEqual(validateRange({ from: '', to: '' }), {
    from: 'Choose a start date.',
    to: 'Choose an end date.',
  });
});

test('validateRange rejects malformed dates', () => {
  assert.deepEqual(validateRange({ from: '2026-1-1', to: '2026-13-01' }), {
    from: 'Choose a start date.',
    to: 'Choose an end date.',
  });
});

test('validateRange rejects impossible calendar dates', () => {
  assert.deepEqual(validateRange({ from: '2026-02-30', to: '2026-03-01' }), {
    from: 'Choose a start date.',
  });
  assert.deepEqual(validateRange({ from: '2026-02-28', to: '2026-02-29' }), {
    to: 'Choose an end date.',
  });
});

test('validateRange accepts a leap day', () => {
  assert.deepEqual(validateRange({ from: '2024-02-29', to: '2024-03-01' }), {});
});

test('formatReportDate formats a UTC calendar date', () => {
  assert.equal(formatReportDate('2026-11-01'), '1 Nov 2026');
  assert.equal(formatReportDate('2026-11-05'), '5 Nov 2026');
  assert.equal(formatReportDate('2026-12-31'), '31 Dec 2026');
});

test('formatReportDate leaves invalid input alone', () => {
  assert.equal(formatReportDate('not-a-date'), 'not-a-date');
  assert.equal(formatReportDate('2026-02-30'), '2026-02-30');
});

test('rangeErrorsFromIssues maps date fields to our wording', () => {
  assert.deepEqual(rangeErrorsFromIssues([{ field: 'from', issue: 'raw' }]), {
    from: 'Choose a valid start date.',
  });
  assert.deepEqual(rangeErrorsFromIssues([{ field: 'to', issue: 'raw' }]), {
    to: 'Choose a valid end date within the 90-day range.',
  });
});

test('rangeErrorsFromIssues falls back when the field is unknown', () => {
  assert.deepEqual(rangeErrorsFromIssues([{ field: 'page', issue: 'raw' }]), {
    to: 'The server did not accept that date range. Change the dates and apply them again.',
  });
});

test('daily report sends the range and returns the rows', async () => {
  const calls = ok(daily);
  const controller = new AbortController();

  assert.deepEqual(await getDailyReport(range, controller.signal), daily);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].method, 'GET');
  assert.equal(
    calls[0].url,
    '/api/v1/reports/daily?from=2026-11-01&to=2026-11-05',
  );
  assert.equal(calls[0].signal, controller.signal);
});

test('outcomes report sends the range and returns the summary', async () => {
  const calls = ok(outcomes);

  assert.deepEqual(await getOutcomesReport(range), outcomes);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].method, 'GET');
  assert.equal(
    calls[0].url,
    '/api/v1/reports/outcomes?from=2026-11-01&to=2026-11-05',
  );
});

test('a 403 becomes FORBIDDEN', async () => {
  fail(403, 'FORBIDDEN');
  await assert.rejects(getDailyReport(range), {
    code: 'FORBIDDEN',
    status: 403,
  });
});

test('a 422 keeps the field issues and our code', async () => {
  stubFetch(422, {
    error: {
      code: 'VALIDATION_ERROR',
      message: 'RAW SERVER WORDS',
      details: [{ field: 'to', issue: 'must not be before from' }],
    },
    meta: { requestId: 'req-1' },
  });

  await assert.rejects(getDailyReport(range), (error) => {
    assert.equal(error.code, 'VALIDATION_FAILED');
    assert.equal(error.status, 422);
    assert.deepEqual(error.issues, [
      { field: 'to', issue: 'must not be before from' },
    ]);
    return true;
  });
});

test('a 429 keeps the retry-after wait', async () => {
  fail(429, 'RATE_LIMITED', { 'Retry-After': '30' });
  await assert.rejects(getDailyReport(range), {
    code: 'RATE_LIMITED',
    retryAfter: 30,
  });
});

test('a 503 becomes INTERNAL', async () => {
  fail(503, 'SERVICE_UNAVAILABLE');
  await assert.rejects(getOutcomesReport(range), {
    code: 'INTERNAL',
    status: 503,
  });
});

test('no connection becomes NETWORK', async () => {
  failNetwork();
  await assert.rejects(getDailyReport(range), { code: 'NETWORK' });
});
