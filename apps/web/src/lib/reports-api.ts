import { api } from './api';

/**
 * The two report calls (API v1.1 sections 7.1 and 7.2) and the date helpers
 * the reports screen needs. Report dates are UTC calendar dates written
 * YYYY-MM-DD, so every helper here works in UTC and never in local time.
 */

export type DailyRow = {
  /** A UTC calendar date, YYYY-MM-DD. */
  date: string;
  created: number;
  delivered: number;
  failed: number;
};

export type OutcomesReport = {
  from: string;
  to: string;
  /** Parcels created in the range, by their status now. */
  totals: {
    created: number;
    delivered: number;
    failed: number;
    inProgress: number;
  };
  /** Percentages of finished parcels (delivered plus failed), 0 to 100. */
  successRate: number;
  failureRate: number;
  topFailureReasons: Array<{ reason: string; count: number }>;
};

export type DateRange = { from: string; to: string };
export type RangeErrors = { from?: string; to?: string };

/** The API rejects a longer range with a 422. */
export const MAX_RANGE_DAYS = 90;
/** The range the screen opens with. */
export const DEFAULT_RANGE_DAYS = 30;

const DAY_MS = 86_400_000;
const DATE_PATTERN = /^\d{4}-\d{2}-\d{2}$/;

function parse(date: string): number | null {
  if (!DATE_PATTERN.test(date)) return null;
  const time = Date.parse(`${date}T00:00:00Z`);
  return Number.isNaN(time) ? null : time;
}

function format(time: number): string {
  return new Date(time).toISOString().slice(0, 10);
}

/** `date` moved by `days` (negative goes back), still YYYY-MM-DD. */
export function shiftDays(date: string, days: number): string {
  const time = parse(date);
  return format((time ?? Date.now()) + days * DAY_MS);
}

/** The last `days` days, ending today in UTC. */
export function defaultRange(
  days = DEFAULT_RANGE_DAYS,
  now: Date = new Date(),
): DateRange {
  const to = format(now.getTime());
  return { from: shiftDays(to, -(days - 1)), to };
}

/**
 * Checks a range before it is sent, with the same rules as the API, and
 * returns a message for each field that is wrong. No messages means it is fine.
 */
export function validateRange({ from, to }: DateRange): RangeErrors {
  const start = parse(from);
  const end = parse(to);
  const errors: RangeErrors = {};
  if (start === null) errors.from = 'Choose a start date.';
  if (end === null) errors.to = 'Choose an end date.';
  if (start === null || end === null) return errors;

  if (start > end) {
    errors.to = 'The end date must be on or after the start date.';
  } else if ((end - start) / DAY_MS + 1 > MAX_RANGE_DAYS) {
    errors.to = `Choose a range of ${MAX_RANGE_DAYS} days or fewer.`;
  }
  return errors;
}

const dateLabel = new Intl.DateTimeFormat('en-GB', {
  dateStyle: 'medium',
  timeZone: 'UTC',
});

/** "4 Nov 2026" from YYYY-MM-DD. The date is UTC, so it never shifts a day. */
export function formatReportDate(date: string): string {
  const time = parse(date);
  return time === null ? date : dateLabel.format(time);
}

function query({ from, to }: DateRange): string {
  return new URLSearchParams({ from, to }).toString();
}

/** API 7.1: one row per date in the range. DISPATCHER and ADMIN. */
export function getDailyReport(
  range: DateRange,
  signal?: AbortSignal,
): Promise<DailyRow[]> {
  return api<DailyRow[]>(`/reports/daily?${query(range)}`, { signal });
}

/** API 7.2: totals, success and failure rates, top failure reasons. DISPATCHER and ADMIN. */
export function getOutcomesReport(
  range: DateRange,
  signal?: AbortSignal,
): Promise<OutcomesReport> {
  return api<OutcomesReport>(`/reports/outcomes?${query(range)}`, { signal });
}