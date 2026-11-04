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
