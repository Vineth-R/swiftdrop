import type { AdminParcelItem } from '../../lib/dispatch-api';

/**
 * A status change received over the socket. The client sequence number tells a
 * change that arrived while a reload was loading from one the reload already
 * contains.
 */
export type LiveChange = {
  status: string;
  /** The server timestamp of the change. */
  at: string;
  /** Client order, increasing with every accepted change. */
  seq: number;
};

/** An update older than the one already held for the parcel changes nothing. */
export function isOlderUpdate(
  update: { updatedAt: string },
  current: LiveChange | undefined,
): boolean {
  return (
    current !== undefined &&
    Date.parse(update.updatedAt) < Date.parse(current.at)
  );
}

/** The row with the live status, if it differs. Only the status is copied. */
export function withLiveStatus(
  row: AdminParcelItem,
  change: LiveChange | undefined,
): AdminParcelItem {
  return change !== undefined && change.status !== row.status
    ? { ...row, status: change.status }
    : row;
}

/**
 * Drop the changes a fresh snapshot supersedes.
 *
 * A change that arrived before the request started is already in the snapshot,
 * because the request reads the database after the event was applied, so it is
 * dropped. A change that arrived while the request was loading may be newer
 * than the snapshot: it is kept only if the snapshot still shows an older
 * status for that row. A change the snapshot already shows is consumed, and a
 * change for a parcel that is not in the snapshot cannot be shown on this page.
 * Either way the pending set stays bounded by the visible page.
 */
export function pruneForSnapshot(
  changes: Map<string, LiveChange>,
  rows: AdminParcelItem[],
  startedAt: number,
): Map<string, LiveChange> {
  const snapshotStatus = new Map(
    rows.map((row) => [row.trackingNumber, row.status]),
  );
  const kept = new Map<string, LiveChange>();
  for (const [trackingNumber, change] of changes) {
    const status = snapshotStatus.get(trackingNumber);
    if (status === undefined) continue; // not on this page
    if (change.seq <= startedAt) continue; // already in the snapshot
    if (status === change.status) continue; // the snapshot already shows it
    kept.set(trackingNumber, change);
  }
  return kept;
}

/** The snapshot with every pending live status applied. */
export function applyLiveStatuses(
  rows: AdminParcelItem[],
  changes: Map<string, LiveChange>,
): AdminParcelItem[] {
  return rows.map((row) =>
    withLiveStatus(row, changes.get(row.trackingNumber)),
  );
}

/**
 * Drop changes for parcels that are no longer on the page. A live view only
 * needs the pending set for the rows it can update, so this keeps it small.
 */
export function pruneToRows(
  changes: Map<string, LiveChange>,
  rows: AdminParcelItem[] | null,
): Map<string, LiveChange> {
  if (rows === null) return changes;
  const onPage = new Set(rows.map((row) => row.trackingNumber));
  const kept = new Map<string, LiveChange>();
  for (const [trackingNumber, change] of changes) {
    if (onPage.has(trackingNumber)) kept.set(trackingNumber, change);
  }
  return kept;
}
