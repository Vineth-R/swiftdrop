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

export function getTracking(
  trackingNumber: string,
  signal?: AbortSignal,
): Promise<Tracking> {
  return api<Tracking>(`/tracking/${encodeURIComponent(trackingNumber)}`, {
    signal,
  });
}
