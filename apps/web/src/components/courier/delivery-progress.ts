import type { CourierHistoryEntry } from '../../lib/courier-api';
import type { ParcelStatus } from '../../lib/tokens';

export const DELIVERY_STAGES = [
  { status: 'CREATED', title: 'Created' },
  { status: 'PICKED_UP', title: 'Picked up' },
  { status: 'IN_TRANSIT', title: 'In transit' },
  { status: 'OUT_FOR_DELIVERY', title: 'Out for delivery' },
  { status: 'DELIVERED', title: 'Delivered' },
] as const satisfies readonly { status: ParcelStatus; title: string }[];

export type ProgressState = 'complete' | 'current' | 'upcoming' | 'failed';

export type ProgressStep = {
  id: string;
  status: ParcelStatus;
  title: string;
  at?: string;
  detail?: string;
  state: ProgressState;
};

function firstAt(
  history: CourierHistoryEntry[],
  status: ParcelStatus,
): string | undefined {
  return history.find((entry) => entry.status === status)?.changedAt;
}

function lastEntry(
  history: CourierHistoryEntry[],
  status: ParcelStatus,
): CourierHistoryEntry | undefined {
  return [...history].reverse().find((entry) => entry.status === status);
}

/**
 * The five-stage delivery route. A delivered parcel marks every stage and the
 * last one as current. A failed parcel keeps the stages it reached and adds
 * FAILED as the alternative terminal outcome; DELIVERED is not shown because
 * it was never reached.
 */
export function deliveryProgress(
  current: ParcelStatus,
  history: CourierHistoryEntry[],
): ProgressStep[] {
  if (current === 'FAILED') {
    const failure = lastEntry(history, 'FAILED');
    const reached = new Set(history.map((entry) => entry.status));
    const stages = DELIVERY_STAGES.slice(0, 4).map<ProgressStep>(
      (stage, index) => {
        const reachedStage = reached.has(stage.status) || index === 0;
        return {
          id: stage.status,
          status: stage.status,
          title: stage.title,
          at: firstAt(history, stage.status),
          state: reachedStage ? 'complete' : 'upcoming',
        };
      },
    );
    stages.push({
      id: 'FAILED',
      status: 'FAILED',
      title: 'Failed',
      at: failure?.changedAt,
      detail: failure?.failureReason ?? undefined,
      state: 'failed',
    });
    return stages;
  }

  const currentIndex = DELIVERY_STAGES.findIndex(
    (stage) => stage.status === current,
  );
  return DELIVERY_STAGES.map<ProgressStep>((stage, index) => ({
    id: stage.status,
    status: stage.status,
    title: stage.title,
    at: firstAt(history, stage.status),
    state:
      index < currentIndex
        ? 'complete'
        : index === currentIndex
          ? 'current'
          : 'upcoming',
  }));
}

/** True when the delivery still accepts a status action. */
export function isActionable(status: ParcelStatus): boolean {
  return status !== 'DELIVERED' && status !== 'FAILED';
}
