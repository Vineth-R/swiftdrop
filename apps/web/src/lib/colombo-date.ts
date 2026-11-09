/**
 * The calendar date in Asia/Colombo, as YYYY-MM-DD. The courier list endpoint
 * filters completed deliveries by this date, so it must not follow the
 * browser's local timezone.
 */
const colomboDate = new Intl.DateTimeFormat('en-GB', {
  timeZone: 'Asia/Colombo',
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
});

export function todayInColombo(now: Date = new Date()): string {
  const parts = colomboDate.formatToParts(now);
  const part = (name: string) =>
    parts.find((item) => item.type === name)?.value;
  return `${part('year')}-${part('month')}-${part('day')}`;
}
