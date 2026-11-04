import type { ReactNode } from 'react';

import { cn } from '../../lib/cn';
import { formatDateTime } from '../../lib/format';

export type TimelineItem = {
  id: string;
  /** What happened, such as "Picked up" or "Approved by an administrator". */
  title: string;
  /** An ISO timestamp. It is shown in Sri Lanka time. */
  at?: string;
  /** A city or a short note. On a public page: cities only, never people (rule 11). */
  detail?: ReactNode;
  /** The latest event. At most one. */
  isCurrent?: boolean;
  /** Not reached yet: drawn hollow, with no time. */
  isUpcoming?: boolean;
};

type TimelineProps = {
  items: TimelineItem[];
  /** Names the list for screen readers, such as "Tracking history". */
  label: string;
  className?: string;
};

/**
 * What happened to a parcel or an order, in the order you pass it (the API's
 * order, usually oldest first). It borrows the brand's route motif: a filled
 * node for what is done, a ringed node for now, a hollow node for what is
 * still to come, joined by a dotted line.
 */
export function Timeline({ items, label, className }: TimelineProps) {
  return (
    <ol aria-label={label} className={cn('flex flex-col', className)}>
      {items.map((item, index) => {
        const isLast = index === items.length - 1;
        return (
          <li
            key={item.id}
            aria-current={item.isCurrent ? 'step' : undefined}
            className="flex gap-4"
          >
            <span aria-hidden="true" className="flex flex-col items-center">
              <span
                className={cn(
                  'mt-1.5 size-3 shrink-0 rounded-full border-2',
                  item.isUpcoming
                    ? 'border-line-strong bg-surface'
                    : item.isCurrent
                      ? 'border-text bg-accent'
                      : 'border-route-node bg-route-node',
                )}
              />
              {!isLast && (
                <span className="my-1 w-0 flex-1 border-l border-dotted border-line-strong" />
              )}
            </span>
            <div
              className={cn(
                'flex min-w-0 flex-col gap-1',
                isLast ? 'pb-0' : 'pb-6',
              )}
            >
              <p
                className={cn(
                  'text-body',
                  item.isUpcoming
                    ? 'text-text-faint'
                    : item.isCurrent
                      ? 'font-semibold'
                      : 'text-text',
                )}
              >
                {item.title}
              </p>
              {item.at && !item.isUpcoming && (
                <time
                  dateTime={item.at}
                  className="font-mono text-caption tabular text-text-muted"
                >
                  {formatDateTime(item.at)}
                </time>
              )}
              {item.detail && (
                <p className="text-body-sm text-text-muted">{item.detail}</p>
              )}
            </div>
          </li>
        );
      })}
    </ol>
  );
}
