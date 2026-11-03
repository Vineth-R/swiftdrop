import type { ReactNode } from 'react';

import { cn } from '../../lib/cn';

type SkeletonProps = {
  /** Size it with token classes such as `h-5 w-1/2`. */
  className?: string;
};

/**
 * One grey block that stands where content will appear. It is still on
 * purpose: the spinner and the route line are the only looping motion in the
 * product. Always place it inside a `SkeletonGroup`, which says what is loading.
 */
export function Skeleton({ className }: SkeletonProps) {
  return (
    <div
      aria-hidden="true"
      className={cn('h-4 rounded-chip bg-surface-3', className)}
    />
  );
}

type SkeletonTextProps = {
  /** How many lines. The last one is shorter, like the end of a paragraph. */
  lines?: number;
  className?: string;
};

/** A paragraph of grey lines. */
export function SkeletonText({ lines = 3, className }: SkeletonTextProps) {
  return (
    <div aria-hidden="true" className={cn('flex flex-col gap-3', className)}>
      {Array.from({ length: lines }, (_, index) => (
        <Skeleton
          key={index}
          className={index === lines - 1 && lines > 1 ? 'w-2/3' : 'w-full'}
        />
      ))}
    </div>
  );
}

type SkeletonGroupProps = {
  /** What is loading, spoken to screen readers, such as "Loading your parcels". */
  label: string;
  children: ReactNode;
  className?: string;
};

/**
 * The loading state of a region. Screen readers hear the label once and the
 * grey blocks are hidden from them.
 */
export function SkeletonGroup({
  label,
  children,
  className,
}: SkeletonGroupProps) {
  return (
    <div role="status" aria-busy="true" className={className}>
      <span className="sr-only">{label}</span>
      {children}
    </div>
  );
}
