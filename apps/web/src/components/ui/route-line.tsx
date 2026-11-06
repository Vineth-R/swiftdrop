import type { CSSProperties } from 'react';

import { cn } from '@/lib/cn';

type RouteLineProps = {
  orientation?: 'horizontal' | 'vertical';
  /**
   * idle: nothing has happened yet. travelling: the lime node runs from origin
   * to destination. complete: the destination is reached.
   */
  state?: 'idle' | 'travelling' | 'complete';
  /** Node diameter in px. The design uses 7 to 10. */
  nodeSize?: number;
  /** Length of one trip in ms. Splash 2400, brand 2600, login panel 3000. */
  cycleMs?: number;
  className?: string;
};

/**
 * The brand motif: a filled origin, a dotted line and a destination that is
 * hollow until reached. It is decorative, so it is hidden from screen readers;
 * put the meaning in nearby text. Under prefers-reduced-motion the travelling
 * node parks at the destination.
 */
export function RouteLine({
  orientation = 'horizontal',
  state = 'idle',
  nodeSize = 9,
  cycleMs = 2600,
  className,
}: RouteLineProps) {
  const isVertical = orientation === 'vertical';
  const node = { width: nodeSize, height: nodeSize };
  const style = { '--route-node': `${nodeSize}px` } as CSSProperties;

  return (
    <span
      aria-hidden="true"
      style={style}
      className={cn(
        'relative flex',
        isVertical ? 'flex-col items-center' : 'items-center',
        className,
      )}
    >
      <span style={node} className="shrink-0 rounded-full bg-route-node" />
      <span
        className={cn(
          'flex-1 border-dotted border-line-strong',
          isVertical ? 'border-l' : 'border-t',
        )}
      />
      <span
        style={node}
        className={cn(
          'box-border shrink-0 rounded-full',
          state === 'complete'
            ? 'border border-ink bg-accent'
            : 'border-[1.5px] border-route-node bg-transparent',
        )}
      />
      {state === 'travelling' && (
        <span
          data-route-dot=""
          data-axis={isVertical ? 'y' : 'x'}
          style={{ ...node, animationDuration: `${cycleMs}ms` }}
          className={cn(
            'absolute box-border rounded-full border border-ink bg-accent',
            isVertical
              ? 'left-1/2 -translate-x-1/2 animate-route-y'
              : 'top-1/2 -translate-y-1/2 animate-route-x',
          )}
        />
      )}
    </span>
  );
}
