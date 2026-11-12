import { describe, expect, it } from 'vitest';

import type { AdminParcelItem } from '../../lib/dispatch-api';
import {
  applyLiveStatuses,
  isOlderUpdate,
  pruneForSnapshot,
  pruneToRows,
  withLiveStatus,
  type LiveChange,
} from './live-merge';

const row = (trackingNumber: string, status = 'CREATED'): AdminParcelItem => ({
  id: trackingNumber,
  trackingNumber,
  status,
  pickupCity: 'Colombo',
  dropoffCity: 'Kandy',
  courierId: null,
  merchantId: null,
  customerId: null,
  total: 450,
  createdAt: '2026-11-12T08:00:00Z',
});

const change = (status: string, at: string, seq: number): LiveChange => ({
  status,
  at,
  seq,
});

describe('isOlderUpdate', () => {
  it('only refuses an update older than the one already held', () => {
    const held = change('OUT_FOR_DELIVERY', '2026-11-12T09:15:00Z', 1);
    expect(isOlderUpdate({ updatedAt: '2026-11-12T09:00:00Z' }, held)).toBe(
      true,
    );
    expect(isOlderUpdate({ updatedAt: '2026-11-12T09:15:00Z' }, held)).toBe(
      false,
    );
    expect(isOlderUpdate({ updatedAt: '2026-11-12T09:30:00Z' }, held)).toBe(
      false,
    );
    expect(
      isOlderUpdate({ updatedAt: '2026-11-12T09:00:00Z' }, undefined),
    ).toBe(false);
  });
});

describe('withLiveStatus', () => {
  it('copies only the status and leaves an unchanged row alone', () => {
    const created = row('SDK7M2Q9XP4');
    const updated = withLiveStatus(
      created,
      change('OUT_FOR_DELIVERY', '2026-11-12T09:15:00Z', 1),
    );
    expect(updated).toEqual({ ...created, status: 'OUT_FOR_DELIVERY' });
    expect(withLiveStatus(created, undefined)).toBe(created);
    expect(
      withLiveStatus(created, change('CREATED', '2026-11-12T09:15:00Z', 1)),
    ).toBe(created);
  });
});

describe('pruneForSnapshot', () => {
  it('keeps only changes that arrived while the request was loading', () => {
    const rows = [row('SDK7M2Q9XP4'), row('SDK2B3C4D5E')];
    const changes = new Map([
      ['SDK7M2Q9XP4', change('OUT_FOR_DELIVERY', '2026-11-12T09:15:00Z', 5)],
      ['SDK2B3C4D5E', change('PICKED_UP', '2026-11-12T09:10:00Z', 4)],
    ]);

    // The snapshot started after sequence 4, so the second change is in it.
    const kept = pruneForSnapshot(changes, rows, 4);
    expect([...kept.keys()]).toEqual(['SDK7M2Q9XP4']);
    expect(changes.size).toBe(2); // the input is not mutated
  });

  it('drops a change the snapshot already shows', () => {
    const kept = pruneForSnapshot(
      new Map([
        ['SDK7M2Q9XP4', change('OUT_FOR_DELIVERY', '2026-11-12T09:15:00Z', 5)],
      ]),
      [row('SDK7M2Q9XP4', 'OUT_FOR_DELIVERY')],
      4,
    );
    expect(kept.size).toBe(0);
  });

  it('drops a change for a parcel that is not on the page', () => {
    const kept = pruneForSnapshot(
      new Map([
        ['SDN9Z8Y7X6W', change('DELIVERED', '2026-11-12T09:20:00Z', 5)],
      ]),
      [row('SDK7M2Q9XP4')],
      0,
    );
    expect(kept.size).toBe(0);
  });
});

describe('applyLiveStatuses', () => {
  it('applies a pending status to its row only', () => {
    const rows = [row('SDK7M2Q9XP4'), row('SDK2B3C4D5E', 'PICKED_UP')];
    const merged = applyLiveStatuses(
      rows,
      new Map([
        ['SDK7M2Q9XP4', change('OUT_FOR_DELIVERY', '2026-11-12T09:15:00Z', 1)],
      ]),
    );
    expect(merged.map((item) => item.status)).toEqual([
      'OUT_FOR_DELIVERY',
      'PICKED_UP',
    ]);
  });
});

describe('pruneToRows', () => {
  it('drops changes whose row left the page', () => {
    const kept = pruneToRows(
      new Map([
        ['SDK7M2Q9XP4', change('OUT_FOR_DELIVERY', '2026-11-12T09:15:00Z', 1)],
        ['SDN9Z8Y7X6W', change('DELIVERED', '2026-11-12T09:20:00Z', 2)],
      ]),
      [row('SDK7M2Q9XP4')],
    );
    expect([...kept.keys()]).toEqual(['SDK7M2Q9XP4']);
  });

  it('leaves the set alone while the page is still loading', () => {
    const changes = new Map([
      ['SDK7M2Q9XP4', change('OUT_FOR_DELIVERY', '2026-11-12T09:15:00Z', 1)],
    ]);
    expect(pruneToRows(changes, null)).toBe(changes);
  });
});
