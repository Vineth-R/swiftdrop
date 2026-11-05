import { api } from './api';
import type { ParcelStatus } from './tokens';

/**
 * Public parcel tracking (API v1.1 section 4.6). No login is needed, so this
 * shape holds only cities, statuses and times: never a name, phone number,
 * address or fee.
 */
export type Tracking = {
  trackingNumber: string;
  status: ParcelStatus;
  pickupCity: string;
  dropoffCity: string;
  lastUpdatedAt: string;
  timeline: { status: ParcelStatus; at: string }[];
};

/**
 * The service only accepts SD[KN] plus 8 capitals or digits. This undoes what
 * typing or pasting adds, and does not check the format: the screen does that.
 */
export function normalizeTrackingNumber(trackingNumber: string): string {
  return trackingNumber.trim().toUpperCase();
}

/**
 * Cleans up the number itself, and copies the public fields by name so that
 * anything else in the response can never reach a public page.
 */
export async function getTracking(
  trackingNumber: string,
  signal?: AbortSignal,
): Promise<Tracking> {
  const number = encodeURIComponent(normalizeTrackingNumber(trackingNumber));
  const data = await api<Tracking>(`/tracking/${number}`, { signal });
  return {
    trackingNumber: data.trackingNumber,
    status: data.status,
    pickupCity: data.pickupCity,
    dropoffCity: data.dropoffCity,
    lastUpdatedAt: data.lastUpdatedAt,
    timeline: data.timeline.map(({ status, at }) => ({ status, at })),
  };
}
