import { formatReportDate, type DailyRow } from '../../lib/reports-api';

/**
 * Delivered and failed parcels for each day, as stacked bars. Failed sits on
 * top of delivered. Created counts are in the table under the chart.
 *
 * The bars and gridlines are SVG shapes coloured with token classes. All text
 * (the legend, the counts on the left and the dates underneath) is ordinary
 * HTML, so it stays readable at any width. The drawing stretches to the width
 * of its container, so it needs no style attribute. The table under it holds
 * every number, and each day has a tooltip for pointer users.
 */

/** Drawing units per day, and the width of one bar inside that slot. */
const SLOT = 10;
const BAR = 6;
const HEIGHT = 100;
/** The counts on the left split the height into this many equal steps. */
const GRID_STEPS = 4;
/** About this many dates are shown under the bars. */
const LABELS_WANTED = 6;

const shortDate = new Intl.DateTimeFormat('en-GB', {
  day: 'numeric',
  month: 'short',
  timeZone: 'UTC',
});

function formatShort(date: string) {
  return shortDate.format(new Date(`${date}T00:00:00Z`));
}

function describeDay(row: DailyRow) {
  const finished = row.delivered + row.failed;
  const rate =
    finished > 0
      ? `, ${Math.round((row.delivered / finished) * 100)}% success`
      : '';
  return `${formatReportDate(row.date)}: ${row.created} created, ${row.delivered} delivered, ${row.failed} failed${rate}`;
}

type DailyChartProps = { rows: DailyRow[] };

export function DailyChart({ rows }: DailyChartProps) {
  if (rows.length === 0) return null;

  const tallest = Math.max(0, ...rows.map((row) => row.delivered + row.failed));
  const hasFinished = tallest > 0;
  // Whole-number steps, so every count on the left is an integer.
  const step = Math.max(1, Math.ceil(tallest / GRID_STEPS));
  const scaleMaximum = step * GRID_STEPS;
  const ticks = Array.from(
    { length: GRID_STEPS + 1 },
    (_, index) => scaleMaximum - index * step,
  );
  const labelEvery = Math.max(1, Math.ceil(rows.length / LABELS_WANTED));
  const width = rows.length * SLOT;
  const first = formatReportDate(rows[0].date);
  const last = formatReportDate(rows[rows.length - 1].date);

  return (
    <figure className="flex flex-col gap-3 rounded-card border border-line bg-surface p-4">
      <figcaption className="flex flex-wrap items-center justify-between gap-3 text-body-sm text-text-muted">
        <span>
          Delivered and failed parcels per day.{' '}
          {hasFinished
            ? `Tallest day: ${tallest}.`
            : 'No parcels finished in this range.'}
        </span>
        <ul className="flex gap-4">
          <li className="flex items-center gap-2">
            <span
              aria-hidden="true"
              className="size-3 rounded-chip bg-success"
            />
            Delivered
          </li>
          <li className="flex items-center gap-2">
            <span
              aria-hidden="true"
              className="size-3 rounded-chip bg-danger"
            />
            Failed
          </li>
        </ul>
      </figcaption>

      <div className="flex gap-2">
        <div
          aria-hidden="true"
          className="flex h-40 w-8 flex-col justify-between text-right text-caption text-text-muted"
        >
          {ticks.map((tick) => (
            <span key={tick} className="leading-none">
              {tick}
            </span>
          ))}
        </div>

        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <svg
            role="img"
            aria-label={`Bar chart of delivered and failed parcels per day from ${first} to ${last}. ${
              hasFinished
                ? `The tallest day has ${tallest} finished parcels.`
                : 'No parcels finished in this range.'
            } The table below lists every number.`}
            viewBox={`0 0 ${width} ${HEIGHT}`}
            preserveAspectRatio="none"
            className="h-40 w-full overflow-visible"
          >
            {ticks.map((tick) => {
              const y = HEIGHT - (tick / scaleMaximum) * HEIGHT;
              return (
                <line
                  key={tick}
                  x1={0}
                  x2={width}
                  y1={y}
                  y2={y}
                  vectorEffect="non-scaling-stroke"
                  className={tick === 0 ? 'stroke-line-strong' : 'stroke-line'}
                />
              );
            })}
            {rows.map((row, index) => {
              const delivered = (row.delivered / scaleMaximum) * HEIGHT;
              const failed = (row.failed / scaleMaximum) * HEIGHT;
              const x = index * SLOT + (SLOT - BAR) / 2;
              return (
                <g key={row.date}>
                  <title>{describeDay(row)}</title>
                  <rect
                    x={index * SLOT}
                    y={0}
                    width={SLOT}
                    height={HEIGHT}
                    className="fill-transparent"
                  />
                  {delivered + failed === 0 && (
                    <rect
                      x={x}
                      y={HEIGHT - 1}
                      width={BAR}
                      height={1}
                      className="fill-line-strong"
                    />
                  )}
                  {delivered > 0 && (
                    <rect
                      x={x}
                      y={HEIGHT - delivered}
                      width={BAR}
                      height={delivered}
                      className="fill-success"
                    />
                  )}
                  {failed > 0 && (
                    <rect
                      x={x}
                      y={HEIGHT - delivered - failed}
                      width={BAR}
                      height={failed}
                      className="fill-danger"
                    />
                  )}
                </g>
              );
            })}
          </svg>

          <div
            aria-hidden="true"
            className="grid h-4 auto-cols-fr grid-flow-col"
          >
            {rows.map((row, index) => (
              <span key={row.date} className="relative">
                {index % labelEvery === 0 && (
                  <span className="absolute left-1/2 -translate-x-1/2 whitespace-nowrap text-caption text-text-muted">
                    {formatShort(row.date)}
                  </span>
                )}
              </span>
            ))}
          </div>
        </div>
      </div>
    </figure>
  );
}
