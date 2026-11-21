import { cn } from '../../lib/cn';
import type { ParcelStatus } from '../../lib/tokens';
import { KeyValueList, type KeyValueItem } from './key-value-list';
import { RouteLine } from './route-line';
import { Skeleton, SkeletonGroup } from './skeleton';
import { StatusChip } from './status-chip';

export type WaybillRow = KeyValueItem & {
  /**
   * Mark a name, phone number, address or fee. A public stub drops these rows,
   * so a page anyone can open can never show them (rule 11).
   */
  isPrivate?: boolean;
};

type WaybillStubProps = {
  trackingNumber: string;
  status: ParcelStatus;
  /** A city. */
  origin: string;
  /** A city. */
  destination: string;
  /** Facts under the tear line, such as the last update or the weight. */
  footRows?: WaybillRow[];
  /** Drops every row marked `isPrivate` and says what the page shows. */
  isPublic?: boolean;
  /** One row for a list of parcels: number, status and the two cities. */
  isCompact?: boolean;
  isLoading?: boolean;
  className?: string;
};

/** Where the route line stands for each status. */
const routeStates = {
  CREATED: 'idle',
  PICKED_UP: 'travelling',
  IN_TRANSIT: 'travelling',
  OUT_FOR_DELIVERY: 'travelling',
  DELIVERED: 'complete',
  FAILED: 'idle',
} as const;

/**
 * A parcel as a paper waybill: the tracking number and status on top, the two
 * cities on the route line, and the details under a tear line. The public
 * version shows what the public tracking page may show and nothing else.
 */
export function WaybillStub({
  trackingNumber,
  status,
  origin,
  destination,
  footRows = [],
  isPublic,
  isCompact,
  isLoading,
  className,
}: WaybillStubProps) {
  if (isLoading) {
    return (
      <SkeletonGroup
        label="Loading parcel details"
        className={cn(
          'flex flex-col gap-4 rounded-card border border-line bg-surface p-4 md:p-6',
          className,
        )}
      >
        <Skeleton className="h-5 w-1/2" />
        <Skeleton className="h-4 w-full" />
        {!isCompact && <Skeleton className="h-10 w-full" />}
      </SkeletonGroup>
    );
  }

  const rows = isPublic ? footRows.filter((row) => !row.isPrivate) : footRows;

  if (isCompact) {
    return (
      <div
        className={cn(
          'flex flex-wrap items-center justify-between gap-x-4 gap-y-2 rounded-card border border-line bg-surface p-3',
          className,
        )}
      >
        <div className="flex min-w-0 flex-col gap-1">
          <span className="font-mono text-body-sm font-semibold tabular">
            {trackingNumber}
          </span>
          <span className="text-body-sm text-text-muted">
            {origin} to {destination}
          </span>
        </div>
        <StatusChip status={status} size="sm" />
      </div>
    );
  }

  return (
    <div
      className={cn(
        'flex flex-col overflow-hidden rounded-card border border-line bg-surface',
        className,
      )}
    >
      <div className="flex flex-col gap-6 p-4 md:p-6">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="flex min-w-0 flex-col gap-1">
            <span className="text-overline uppercase text-text-faint">
              Tracking number
            </span>
            <span className="break-all font-mono text-h4 tabular">
              {trackingNumber}
            </span>
          </div>
          <StatusChip status={status} />
        </div>
        <div className="flex items-center gap-4">
          <span className="text-label">{origin}</span>
          <RouteLine state={routeStates[status]} className="flex-1" />
          <span className="text-label">{destination}</span>
        </div>
      </div>
      {(rows.length > 0 || isPublic) && (
        <div className="flex flex-1 flex-col gap-4 border-t border-dashed border-line-strong bg-surface-2 p-4 md:p-6">
          {rows.length > 0 && <KeyValueList items={rows} layout="inline" />}
          {isPublic && (
            <p className="text-caption text-text-faint">
              Only the status, the cities and the times are shown here.
            </p>
          )}
        </div>
      )}
    </div>
  );
}
