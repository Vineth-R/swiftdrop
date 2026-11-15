import { describe, expect, it } from 'vitest';

import type { CourierHistoryEntry } from '../../lib/courier-api';
import type { ParcelStatus } from '../../lib/tokens';
import {
  deliveryProgress,
  isActionable,
  DELIVERY_STAGES,
} from './delivery-progress';

function entry(
  status: ParcelStatus,
  changedAt: string,
  extra: Partial<CourierHistoryEntry> = {},
): CourierHistoryEntry {
  return {
    previousStatus: null,
    status,
    note: null,
    failureReason: null,
    actorRole: 'COURIER',
    changedAt,
    ...extra,
  };
}

describe('deliveryProgress', () => {
  it('marks completed, current and upcoming stages for an active parcel', () => {
    const steps = deliveryProgress('IN_TRANSIT', [
      entry('CREATED', '2026-11-05T08:00:00Z'),
      entry('PICKED_UP', '2026-11-05T09:00:00Z'),
      entry('IN_TRANSIT', '2026-11-05T09:00:01Z'),
    ]);
    expect(steps.map((step) => step.state)).toEqual([
      'complete',
      'complete',
      'current',
      'upcoming',
      'upcoming',
    ]);
    expect(steps[0].at).toBe('2026-11-05T08:00:00Z');
    expect(steps[2].at).toBe('2026-11-05T09:00:01Z');
    expect(steps[4].at).toBeUndefined();
  });

  it('shows every stage and marks the last one current when delivered', () => {
    const steps = deliveryProgress('DELIVERED', [
      entry('CREATED', '2026-11-05T08:00:00Z'),
      entry('PICKED_UP', '2026-11-05T09:00:00Z'),
      entry('IN_TRANSIT', '2026-11-05T09:00:01Z'),
      entry('OUT_FOR_DELIVERY', '2026-11-05T10:00:00Z'),
      entry('DELIVERED', '2026-11-05T11:00:00Z'),
    ]);
    expect(steps.map((step) => step.status)).toEqual(
      DELIVERY_STAGES.map((stage) => stage.status),
    );
    expect(steps.at(-1)?.state).toBe('current');
    expect(steps.at(-1)?.at).toBe('2026-11-05T11:00:00Z');
  });

  it('adds FAILED as the alternative terminal outcome with its reason', () => {
    const steps = deliveryProgress('FAILED', [
      entry('CREATED', '2026-11-05T08:00:00Z'),
      entry('PICKED_UP', '2026-11-05T09:00:00Z'),
      entry('IN_TRANSIT', '2026-11-05T09:00:01Z'),
      entry('FAILED', '2026-11-05T12:00:00Z', {
        failureReason: 'Recipient not available',
      }),
    ]);
    expect(steps.map((step) => step.status)).toEqual([
      'CREATED',
      'PICKED_UP',
      'IN_TRANSIT',
      'OUT_FOR_DELIVERY',
      'FAILED',
    ]);
    expect(steps[3].state).toBe('upcoming');
    expect(steps[4]).toMatchObject({
      state: 'failed',
      at: '2026-11-05T12:00:00Z',
      detail: 'Recipient not available',
    });
    expect(steps.some((step) => step.status === 'DELIVERED')).toBe(false);
  });

  it('keeps every stage upcoming except the current one without history', () => {
    const steps = deliveryProgress('CREATED', []);
    expect(steps.map((step) => step.state)).toEqual([
      'current',
      'upcoming',
      'upcoming',
      'upcoming',
      'upcoming',
    ]);
  });
});

describe('isActionable', () => {
  it('is true until the parcel reaches a terminal status', () => {
    expect(isActionable('CREATED')).toBe(true);
    expect(isActionable('PICKED_UP')).toBe(true);
    expect(isActionable('IN_TRANSIT')).toBe(true);
    expect(isActionable('OUT_FOR_DELIVERY')).toBe(true);
    expect(isActionable('DELIVERED')).toBe(false);
    expect(isActionable('FAILED')).toBe(false);
  });
});
