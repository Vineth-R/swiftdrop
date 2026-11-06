import type { LucideIcon } from 'lucide-react';
import type { ReactNode } from 'react';

import { cn } from '../../lib/utils';
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
} from './empty';

type EmptyStateProps = {
  /** What is missing, in one short line: "No parcels yet". */
  title: string;
  /** One sentence that says what to do about it. */
  children?: ReactNode;
  /** The one action that fixes it, such as a primary `ButtonLink`. */
  action?: ReactNode;
  /** A Lucide icon with `strokeWidth` 1.5. It must not repeat the title. */
  icon?: LucideIcon;
  className?: string;
};

/**
 * What a list, table or section shows when there is nothing in it yet. It says
 * what is missing and offers the one action that fills it. Use it inside a
 * `Section`, so its title is a third level heading under that section's.
 */
export function EmptyState({
  title,
  children,
  action,
  icon: Icon,
  className,
}: EmptyStateProps) {
  return (
    <Empty
      className={cn(
        'gap-4 rounded-card border border-line bg-surface px-6 py-12',
        className,
      )}
    >
      <EmptyHeader className="max-w-md">
        {Icon && (
          <EmptyMedia
            variant="icon"
            aria-hidden="true"
            className="size-12 rounded-full bg-surface-2 text-text-muted"
          >
            <Icon size={24} strokeWidth={1.5} />
          </EmptyMedia>
        )}
        <h3 className="text-h4">{title}</h3>
        {children && (
          <EmptyDescription className="text-body text-text-muted">
            {children}
          </EmptyDescription>
        )}
      </EmptyHeader>
      {action && <EmptyContent className="max-w-none">{action}</EmptyContent>}
    </Empty>
  );
}
