import { formatReportDate, type DailyRow } from '../../lib/reports-api';

/**
 * Delivered and failed parcels for each day, as stacked bars. Failed sits on
 * top of delivered. Created counts are in the table under the chart.
 *
 * The bars are SVG shapes coloured with token classes. All text (the legend and
 * the first and last dates) is ordinary HTML, so it stays readable at any
 * width. The drawing stretches to the width of its container, so it needs no
 * style attribute. The table under it holds every number, and each bar has a
 * tooltip for pointer users.
 */

/** Drawing units per day, and the width of one bar inside that slot. */
const SLOT = 10;
const BAR = 6;
const HEIGHT = 100;

type DailyChartProps = { rows: DailyRow[] };

export function DailyChart({ rows }: DailyChartProps) {
  if (rows.length === 0) return null;

  const tallest = Math.max(0, ...rows.map((row) => row.delivered + row.failed));
  const scaleMaximum = Math.max(1, tallest);
  const width = rows.length * SLOT;
  const first = formatReportDate(rows[0].date);
  const last = formatReportDate(rows[rows.length - 1].date);
  const hasFinished = tallest > 0;

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

      <svg
        role="img"
        aria-label={`Bar chart of delivered and failed parcels per day from ${first} to ${last}. ${
          hasFinished
            ? `The tallest day has ${tallest} finished parcels.`
            : 'No parcels finished in this range.'
        } The table below lists every number.`}
        viewBox={`0 0 ${width} ${HEIGHT}`}
        preserveAspectRatio="none"
        className="h-40 w-full"
      >
        <line
          x1={0}
          x2={width}
          y1={HEIGHT}
          y2={HEIGHT}
          vectorEffect="non-scaling-stroke"
          className="stroke-line-strong"
        />
        {rows.map((row, index) => {
          const delivered = (row.delivered / scaleMaximum) * HEIGHT;
          const failed = (row.failed / scaleMaximum) * HEIGHT;
          const x = index * SLOT + (SLOT - BAR) / 2;
          return (
            <g key={row.date}>
              <title>{`${formatReportDate(row.date)}: ${row.delivered} delivered, ${row.failed} failed`}</title>
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

      <div className="flex justify-between text-caption text-text-muted">
        <span>{first}</span>
        <span>{last}</span>
      </div>
    </figure>
  );
}
