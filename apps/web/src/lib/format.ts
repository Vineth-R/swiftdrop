/**
 * Dates and times as people read them. Everything is shown in Sri Lanka time
 * with a fixed locale, so the server render and the browser always agree and
 * a parcel's timeline reads the same on every device.
 */
const dateTime = new Intl.DateTimeFormat('en-GB', {
  dateStyle: 'medium',
  timeStyle: 'short',
  timeZone: 'Asia/Colombo',
});

const date = new Intl.DateTimeFormat('en-GB', {
  dateStyle: 'medium',
  timeZone: 'Asia/Colombo',
});

const money = new Intl.NumberFormat('en-GB', {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});

/** "LKR 550.00". */
export function formatLkr(amount: number): string {
  return `LKR ${money.format(amount)}`;
}

/** "2.50 kg". */
export function formatKg(weight: number): string {
  return `${money.format(weight)} kg`;
}

/** "4 Nov 2026, 14:05" from an ISO timestamp. An unreadable value gives an empty string. */
export function formatDateTime(iso: string): string {
  const value = new Date(iso);
  return Number.isNaN(value.getTime()) ? '' : dateTime.format(value);
}

/** "4 Nov 2026" from an ISO timestamp. */
export function formatDate(iso: string): string {
  const value = new Date(iso);
  return Number.isNaN(value.getTime()) ? '' : date.format(value);
}
