'use client';

import { useEffect, useState, type ReactNode } from 'react';

import { ApiError } from '../../lib/api';
import { formatDateTime, formatKg } from '../../lib/format';
import {
  getParcel,
  getParcelAssignments,
  getParcelHistory,
  type Parcel,
  type ParcelAssignment,
  type ParcelHistory,
} from '../../lib/parcels-api';
import { listZones } from '../../lib/dispatch-api';
import type { Zone } from '../../lib/pricing-api';
import { Alert } from '../ui/alert';
import { Button } from '../ui/button';
import { EmptyState } from '../ui/empty-state';
import { KeyValueList } from '../ui/key-value-list';
import { Modal } from '../ui/modal';
import { SkeletonGroup, SkeletonText } from '../ui/skeleton';
import { StatusChip, statusLabel } from '../ui/status-chip';
import { Timeline } from '../ui/timeline';
import { FeeBreakdown } from './fee-breakdown';

type State =
  | { status: 'loading' }
  | {
      status: 'ready';
      parcel: Parcel;
      history: ParcelHistory[];
      assignments: ParcelAssignment[];
    }
  | { status: 'error'; error: ApiError };

function asApiError(error: unknown): ApiError {
  return error instanceof ApiError
    ? error
    : new ApiError({ code: 'INTERNAL', status: 0 });
}

function Group({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-3 border-t border-line pt-5 first:border-t-0 first:pt-0">
      <h3 className="text-h4">{title}</h3>
      {children}
    </section>
  );
}

export type ParcelAssignTarget = {
  id: string;
  trackingNumber: string;
  currentCourierId?: string | null;
};

type ParcelDetailsModalProps = {
  parcelId: string | null;
  onClose: () => void;
  /** Only offered when the parcel can still be assigned. */
  onAssign?: (parcel: ParcelAssignTarget) => void;
};

/**
 * One parcel in full: identifiers, contacts, fee, status history and courier
 * assignment history. It is shared by the Unassigned and All parcels screens.
 */
export function ParcelDetailsModal({
  parcelId,
  onClose,
  onAssign,
}: ParcelDetailsModalProps) {
  const [state, setState] = useState<State>({ status: 'loading' });
  const [reload, setReload] = useState(0);
  const [zones, setZones] = useState<Zone[]>([]);

  useEffect(() => {
    if (!parcelId) return;
    const controller = new AbortController();
    setState({ status: 'loading' });
    Promise.all([
      getParcel(parcelId, controller.signal),
      getParcelHistory(parcelId, controller.signal),
      getParcelAssignments(parcelId, controller.signal),
    ])
      .then(([parcel, history, assignments]) => {
        if (!controller.signal.aborted) {
          setState({ status: 'ready', parcel, history, assignments });
        }
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) {
          setState({ status: 'error', error: asApiError(error) });
        }
      });
    return () => controller.abort();
  }, [parcelId, reload]);

  useEffect(() => {
    if (!parcelId) return;
    const controller = new AbortController();
    listZones(controller.signal)
      .then((list) => {
        if (!controller.signal.aborted) setZones(list);
      })
      .catch(() => {
        if (!controller.signal.aborted) setZones([]);
      });
    return () => controller.abort();
  }, [parcelId]);

  const parcel = state.status === 'ready' ? state.parcel : null;
  const zoneName = (id: string) =>
    zones.find((zone) => zone.id === id)?.name ?? id;
  const isClosed =
    parcel?.status === 'DELIVERED' || parcel?.status === 'FAILED';
  const lastStatusChange =
    state.status === 'ready' ? state.history.at(-1)?.changedAt : undefined;

  return (
    <Modal
      isOpen={parcelId !== null}
      onClose={onClose}
      title={parcel ? parcel.trackingNumber : 'Parcel details'}
      description={
        parcel
          ? 'Parcel information, contacts, fee, status history and assignments.'
          : 'Loading the parcel.'
      }
      size="xl"
      actions={
        <>
          <Button variant="secondary" onClick={onClose}>
            Close
          </Button>
          {parcel && onAssign && !isClosed ? (
            <Button
              onClick={() =>
                onAssign({
                  id: parcel.id,
                  trackingNumber: parcel.trackingNumber,
                  currentCourierId: parcel.courier?.id ?? null,
                })
              }
            >
              {parcel.courier ? 'Reassign courier' : 'Assign courier'}
            </Button>
          ) : null}
        </>
      }
    >
      {state.status === 'loading' ? (
        <SkeletonGroup label="Loading parcel details">
          <div className="flex flex-col gap-4">
            <SkeletonText />
            <SkeletonText />
            <SkeletonText />
          </div>
        </SkeletonGroup>
      ) : state.status === 'error' ? (
        <Alert
          tone="danger"
          title={
            state.error.code === 'NOT_FOUND'
              ? 'This parcel no longer exists'
              : 'Could not load this parcel'
          }
          actions={
            <Button
              variant="secondary"
              size="sm"
              onClick={() => setReload((value) => value + 1)}
            >
              Try again
            </Button>
          }
        >
          {state.error.code === 'FORBIDDEN'
            ? 'Only dispatchers and administrators can open parcel details.'
            : state.error.code === 'NOT_FOUND'
              ? 'Close this dialog and reload the list.'
              : 'Try again in a moment.'}
        </Alert>
      ) : (
        parcel && (
          <div className="flex flex-col gap-5">
            {parcel.failureReason ? (
              <Alert tone="warning" title="Delivery could not be completed">
                {parcel.failureReason}
              </Alert>
            ) : null}

            <Group title="Parcel information">
              <KeyValueList
                layout="inline"
                items={[
                  {
                    label: 'Tracking number',
                    value: parcel.trackingNumber,
                    isMono: true,
                  },
                  { label: 'Parcel ID', value: parcel.id, isMono: true },
                  {
                    label: 'Current status',
                    value: <StatusChip status={parcel.status} />,
                  },
                  {
                    label: 'Created',
                    value: formatDateTime(parcel.createdAt),
                    isMono: true,
                  },
                  {
                    label: 'Last updated',
                    value: formatDateTime(parcel.updatedAt),
                    isMono: true,
                  },
                  {
                    label: 'Delivered',
                    value: parcel.deliveredAt
                      ? formatDateTime(parcel.deliveredAt)
                      : null,
                    isMono: true,
                  },
                ]}
              />
            </Group>

            <Group title="Pickup information">
              <KeyValueList
                layout="inline"
                items={[
                  { label: 'Address', value: parcel.sender.address },
                  { label: 'City', value: parcel.sender.city },
                  {
                    label: 'Pickup zone',
                    value: `${zoneName(parcel.sender.zoneId)} (${parcel.sender.zoneId})`,
                  },
                  { label: 'Contact name', value: parcel.sender.name },
                  {
                    label: 'Contact phone',
                    value: parcel.sender.phone,
                    isMono: true,
                  },
                ]}
              />
            </Group>

            <Group title="Drop-off information">
              <KeyValueList
                layout="inline"
                items={[
                  { label: 'Address', value: parcel.recipient.address },
                  { label: 'City', value: parcel.recipient.city },
                  {
                    label: 'Destination zone',
                    value: `${zoneName(parcel.recipient.zoneId)} (${parcel.recipient.zoneId})`,
                  },
                  { label: 'Recipient name', value: parcel.recipient.name },
                  {
                    label: 'Recipient phone',
                    value: parcel.recipient.phone,
                    isMono: true,
                  },
                ]}
              />
            </Group>

            <Group title="Parcel specifications">
              <KeyValueList
                layout="inline"
                items={[
                  {
                    label: 'Weight',
                    value: formatKg(parcel.weightKg),
                    isMono: true,
                  },
                  { label: 'Size', value: parcel.sizeCategory },
                  { label: 'Description', value: parcel.description },
                  {
                    label: 'Assigned courier',
                    value: parcel.courier
                      ? `${parcel.courier.fullName} · ${parcel.courier.phone}`
                      : 'Not assigned yet',
                  },
                ]}
              />
            </Group>

            <Group title="Pricing information">
              <FeeBreakdown fee={parcel.fee} />
            </Group>

            <Group title="Courier assignments">
              {state.assignments.length === 0 ? (
                <EmptyState title="No courier assignment yet">
                  This parcel is waiting for a courier.
                </EmptyState>
              ) : (
                <div className="flex flex-col gap-3">
                  {state.assignments.map((assignment) => (
                    <div
                      key={assignment.id}
                      className="rounded-card border border-line bg-surface p-4"
                    >
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <span className="font-mono text-body-sm">
                          {assignment.courierId}
                        </span>
                        {assignment.isCurrent ? (
                          <span className="rounded-chip border border-line bg-surface-2 px-2 py-1 text-caption text-text-muted">
                            Current assignment
                          </span>
                        ) : null}
                      </div>
                      <KeyValueList
                        className="mt-3"
                        items={[
                          {
                            label: 'Assigned',
                            value: formatDateTime(assignment.assignedAt),
                            isMono: true,
                          },
                          {
                            label: 'Unassigned',
                            value: assignment.unassignedAt
                              ? formatDateTime(assignment.unassignedAt)
                              : null,
                            isMono: true,
                          },
                          {
                            label: 'Assigned by',
                            value: assignment.assignedBy,
                            isMono: true,
                          },
                          {
                            label: 'Previous courier',
                            value: assignment.previousCourierId,
                            isMono: true,
                          },
                          { label: 'Reason', value: assignment.reason },
                        ]}
                      />
                    </div>
                  ))}
                </div>
              )}
            </Group>

            <Group title="Status history">
              {state.history.length === 0 ? (
                <EmptyState title="No status updates yet">
                  Updates appear here as the parcel moves.
                </EmptyState>
              ) : (
                <Timeline
                  label="Parcel status history"
                  items={state.history.map((entry, index) => ({
                    id: `${entry.changedAt}-${index}`,
                    title: statusLabel(entry.status),
                    at: entry.changedAt,
                    detail:
                      entry.note || entry.failureReason ? (
                        <span className="break-words">
                          {[entry.note, entry.failureReason]
                            .filter(Boolean)
                            .join(' · ')}
                        </span>
                      ) : undefined,
                    isCurrent: index === state.history.length - 1,
                  }))}
                />
              )}
            </Group>

            <Group title="Operational notes">
              <KeyValueList
                items={[
                  { label: 'Failure reason', value: parcel.failureReason },
                  {
                    label: 'Last status change',
                    value: lastStatusChange
                      ? formatDateTime(lastStatusChange)
                      : null,
                    isMono: true,
                  },
                ]}
              />
            </Group>
          </div>
        )
      )}
    </Modal>
  );
}
