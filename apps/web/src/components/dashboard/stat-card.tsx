import Link from 'next/link';

import type { Load } from '@/lib/use-load';
import { Skeleton, SkeletonGroup } from '../ui/skeleton';

/**
 * One number a role cares about, with the screen that acts on it. It owns its
 * four states: grey blocks while loading, the number when ready, a plain
 * sentence and a retry when the call failed, and a "nothing waiting" line for
 * zero, so a screen reader hears what the number means either way.
 */
export function StatCard({
  label,
  state,
  onRetry,
  href,
  linkLabel,
  zeroText,
}: {
  label: string;
  /** The count, once it is known. */
  state: Load<number>;
  onRetry: () => void;
  href: string;
  linkLabel: string;
  /** What zero means, such as "No applications are waiting." */
  zeroText: string;
}) {
  return (
    <div className="flex flex-col gap-3 rounded-card border border-line bg-surface p-5">
      <p className="text-overline uppercase text-text-muted">{label}</p>
      {state.status === 'loading' ? (
        <SkeletonGroup label={`Loading ${label.toLowerCase()}`}>
          <Skeleton className="h-8 w-16" />
        </SkeletonGroup>
      ) : state.status === 'error' ? (
        <div className="flex flex-col items-start gap-2">
          <p className="text-body-sm text-text-muted">
            This could not be shown right now.
          </p>
          <button
            type="button"
            onClick={onRetry}
            className="text-body-sm font-semibold text-interactive underline underline-offset-4 pointer-coarse:min-h-11"
          >
            Try again
          </button>
        </div>
      ) : (
        <>
          <p className="font-mono text-h2 tabular">{state.data}</p>
          {state.data === 0 ? (
            <p className="text-body-sm text-text-muted">{zeroText}</p>
          ) : null}
        </>
      )}
      <Link
        href={href}
        className="mt-auto text-body-sm font-semibold text-interactive underline underline-offset-4 pointer-coarse:inline-flex pointer-coarse:min-h-11 pointer-coarse:items-center"
      >
        {linkLabel}
      </Link>
    </div>
  );
}
