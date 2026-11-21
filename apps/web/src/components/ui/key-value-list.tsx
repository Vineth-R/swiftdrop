import type { ReactNode } from 'react';

import { cn } from '../../lib/cn';

export type KeyValueItem = {
  label: string;
  /** Empty (null, undefined or '') shows a dash that screen readers read as "Not provided". */
  value: ReactNode;
  /** For identifiers and amounts: mono, tabular (rule 03). */
  isMono?: boolean;
};

type KeyValueListProps = {
  items: KeyValueItem[];
  /**
   * stacked: the label above its value, for narrow places such as a card.
   * inline: the label beside its value from 768px, for a detail page.
   */
  layout?: 'stacked' | 'inline';
  className?: string;
};

function isEmpty(value: ReactNode) {
  return value === null || value === undefined || value === '';
}

/**
 * Facts about one thing: a merchant's profile, an order's details, a receipt's
 * rows. It is a real description list, so a screen reader pairs each label
 * with its value.
 */
export function KeyValueList({
  items,
  layout = 'stacked',
  className,
}: KeyValueListProps) {
  return (
    <dl className={cn('flex flex-col gap-3', className)}>
      {items.map((item) => (
        <div
          key={item.label}
          className={cn(
            'flex flex-col gap-1',
            layout === 'inline' && 'md:flex-row md:gap-4',
          )}
        >
          <dt
            className={cn(
              'text-caption text-text-muted',
              layout === 'inline' && 'md:w-48 md:shrink-0 md:pt-0.5',
            )}
          >
            {item.label}
          </dt>
          <dd
            className={cn(
              'min-w-0 break-words text-body',
              item.isMono && 'font-mono text-body-sm tabular',
            )}
          >
            {isEmpty(item.value) ? (
              <>
                <span aria-hidden="true" className="text-text-faint">
                  —
                </span>
                <span className="sr-only">Not provided</span>
              </>
            ) : (
              item.value
            )}
          </dd>
        </div>
      ))}
    </dl>
  );
}
