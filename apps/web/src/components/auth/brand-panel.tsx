import { ArrowRight } from 'lucide-react';
import type { CSSProperties, ReactNode } from 'react';

import { RouteLine } from '../ui/route-line';

/**
 * The left side of every auth page. It sits inside an element that opts into
 * the dark tokens (see AuthLayout), so every colour below reads correctly on
 * Ink whether the page itself is light or dark.
 *
 * It is made of three things, top to bottom: a map of the network, a waybill
 * ticket like the one a customer gets back from tracking, and one line that
 * says what the page is for. None of it is data. It is the brand: points
 * joined by lines, with one lime node on the move.
 */

type City = {
  name: string;
  x: number;
  y: number;
  anchor: 'start' | 'end' | 'middle';
  dx: number;
  dy: number;
};

const hub = { x: 150, y: 250 };

const cities: City[] = [
  { name: 'JAFFNA', x: 110, y: 60, anchor: 'start', dx: 14, dy: 4 },
  { name: 'KANDY', x: 380, y: 110, anchor: 'start', dx: 14, dy: 4 },
  { name: 'NEGOMBO', x: 300, y: 230, anchor: 'start', dx: 4, dy: -14 },
  { name: 'GALLE', x: 230, y: 350, anchor: 'middle', dx: 0, dy: 24 },
  { name: 'MATARA', x: 450, y: 340, anchor: 'start', dx: 14, dy: 4 },
];

/** Each route is drawn dotted, and a lime streak runs along it in turn. */
const routes = [
  { d: 'M150 250 Q70 150 110 60', delay: 0 },
  { d: 'M150 250 Q270 200 380 110', delay: 1.3 },
  { d: 'M150 250 Q230 215 300 230', delay: 2.6 },
  { d: 'M150 250 Q165 320 230 350', delay: 0.7 },
  { d: 'M230 350 Q340 390 450 340', delay: 3.4 },
  { d: 'M300 230 Q410 250 450 340', delay: 2 },
];

function RouteMap() {
  return (
    <svg
      viewBox="40 24 490 390"
      preserveAspectRatio="xMidYMid meet"
      role="img"
      aria-label="A network of routes between Jaffna, Kandy, Negombo, Colombo, Galle and Matara"
      className="absolute inset-0 h-full w-full"
    >
      {[80, 150, 230].map((radius) => (
        <circle
          key={radius}
          cx={hub.x}
          cy={hub.y}
          r={radius}
          fill="none"
          strokeWidth="1"
          className="stroke-line-strong/70"
        />
      ))}
      {routes.map((route) => (
        <path
          key={route.d}
          d={route.d}
          fill="none"
          strokeWidth="1.5"
          strokeLinecap="round"
          strokeDasharray="1.5 7"
          className="stroke-route-node/60"
        />
      ))}
      {routes.map((route) => (
        <path
          key={`${route.d}-run`}
          data-comet=""
          d={route.d}
          pathLength={100}
          fill="none"
          strokeWidth="3"
          strokeLinecap="round"
          strokeDasharray="7 100"
          style={{ animationDelay: `${route.delay}s` }}
          className="animate-comet stroke-accent [filter:drop-shadow(0_0_3px_var(--color-accent))]"
        />
      ))}

      {[0, 1.6].map((delay) => (
        <circle
          key={delay}
          data-ring=""
          cx={hub.x}
          cy={hub.y}
          r="7"
          fill="none"
          strokeWidth="1"
          style={
            {
              animationDelay: `${delay}s`,
              transformBox: 'fill-box',
              transformOrigin: 'center',
            } as CSSProperties
          }
          className="animate-ring stroke-accent"
        />
      ))}
      <circle cx={hub.x} cy={hub.y} r="6" className="fill-accent" />
      <text
        x={hub.x - 14}
        y={hub.y + 4}
        textAnchor="end"
        className="fill-text text-body-sm font-semibold [letter-spacing:0.09em]"
      >
        COLOMBO
      </text>

      {cities.map((city) => (
        <g key={city.name}>
          <circle
            cx={city.x}
            cy={city.y}
            r="4.5"
            strokeWidth="1.5"
            className="fill-ink stroke-route-node"
          />
          <text
            x={city.x + city.dx}
            y={city.y + city.dy}
            textAnchor={city.anchor}
            className="fill-text-muted text-body-sm font-semibold [letter-spacing:0.09em]"
          >
            {city.name}
          </text>
        </g>
      ))}
    </svg>
  );
}

const overline = 'text-overline uppercase text-text-faint';

/** The stub a customer gets from tracking, drawn the way the design draws it. */
function WaybillTicket() {
  return (
    <div
      aria-hidden="true"
      className="relative w-full max-w-sm overflow-hidden rounded-card border border-line-strong bg-surface"
    >
      <div className="flex flex-col gap-4 px-5 pb-4 pt-4">
        <div className="flex items-start justify-between gap-4">
          <div className="flex min-w-0 flex-col gap-1">
            <span className={overline}>Tracking number</span>
            <span className="font-mono text-h4 font-semibold tracking-[0.02em]">
              SDK7M2Q9XP4
            </span>
          </div>
          <span className="inline-flex h-6 shrink-0 items-center gap-1.5 rounded-chip border border-status-in-transit-bd bg-status-in-transit-bg px-2 text-overline font-semibold text-status-in-transit-fg">
            <ArrowRight size={12} strokeWidth={1.75} />
            IN TRANSIT
          </span>
        </div>
        <div className="flex items-center gap-3">
          <div className="flex flex-col gap-0.5">
            <span className={overline}>From</span>
            <span className="text-body font-semibold">Colombo</span>
          </div>
          <RouteLine
            state="travelling"
            nodeSize={7}
            cycleMs={3600}
            className="mt-3 min-w-10 flex-1"
          />
          <div className="flex flex-col items-end gap-0.5">
            <span className={overline}>To</span>
            <span className="text-body font-semibold">Kandy</span>
          </div>
        </div>
      </div>

      {/* The perforation: a dashed line with a bite taken out of each end. */}
      <div className="relative h-0 border-t border-dashed border-line-strong">
        <span className="absolute -left-2 -top-2 size-4 -rotate-45 rounded-full border border-line-strong border-b-transparent border-l-transparent bg-ink" />
        <span className="absolute -right-2 -top-2 size-4 -rotate-45 rounded-full border border-line-strong border-r-transparent border-t-transparent bg-ink" />
      </div>

      <div className="flex justify-between gap-4 bg-surface-2 px-5 py-3">
        <div className="flex flex-col gap-0.5">
          <span className={overline}>Last scan</span>
          <span className="tabular font-mono text-body-sm">06:40 Kegalle</span>
        </div>
        <div className="flex flex-col items-end gap-0.5">
          <span className={overline}>Expected</span>
          <span className="text-body-sm">Today, before 18:00</span>
        </div>
      </div>
    </div>
  );
}

type BrandPanelProps = {
  /** The one line. Short, plain, about the person and their parcel. */
  headline: string;
  /** One sentence under it. */
  children?: ReactNode;
};

export function BrandPanel({ headline, children }: BrandPanelProps) {
  return (
    <div className="relative flex min-h-0 flex-1 flex-col gap-6">
      {/* A faint dot grid that fades out toward the edges, and a soft glow. */}
      <div
        aria-hidden="true"
        className="pointer-events-none absolute -inset-12 bg-[radial-gradient(var(--color-line-strong)_1px,transparent_1.4px)] bg-[length:22px_22px] opacity-60 [mask-image:radial-gradient(ellipse_75%_60%_at_60%_38%,#000_25%,transparent_100%)]"
      />
      <div
        aria-hidden="true"
        className="pointer-events-none absolute -inset-12 bg-[radial-gradient(ellipse_50%_40%_at_62%_30%,color-mix(in_oklab,var(--color-accent)_6%,transparent),transparent)]"
      />

      <div className="relative min-h-48 flex-1">
        <RouteMap />
      </div>

      <div className="relative -mt-4">
        <WaybillTicket />
      </div>

      <div className="relative flex flex-col gap-3">
        <h2 className="max-w-[16ch] text-balance font-display text-h1 xl:text-display">
          {headline}
        </h2>
        {children && (
          <p className="max-w-[40ch] text-body text-text-muted">{children}</p>
        )}
      </div>
    </div>
  );
}
