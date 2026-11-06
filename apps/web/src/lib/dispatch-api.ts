import { api, apiWithMeta, type ApiMeta } from './api';
import type { Zone } from './pricing-api';

export type { Zone };

/** Default and maximum page size for dispatch pagination. */
export const DISPATCH_PAGE_SIZE = 20;

/** An unassigned parcel waiting for courier assignment (API v1.1 section 5.1). */
export type UnassignedParcel = {
  id: string;
  trackingNumber: string;
  status: string;
  pickupAddress: string;
  pickupCity: string;
  pickupZoneId: string;
  dropoffAddress: string;
  dropoffCity: string;
  dropoffZoneId: string;
  weightKg: number;
  sizeCategory: string;
  createdAt: string;
};

/** Filter and pagination parameters for GET /dispatch/unassigned. */
export type UnassignedQuery = {
  zoneId?: string;
  from?: string;
  to?: string;
  page?: number;
  pageSize?: number;
  sort?: string;
};

/** Request payload for PUT /dispatch/parcels/{parcelId}/assignment (API v1.1 section 5.2). */
export type AssignCourierInput = {
  courierId: string;
  reason?: string;
};

/** Successful result of parcel assignment. */
export type AssignCourierResult = {
  parcelId: string;
  trackingNumber: string;
  courierId: string;
  previousCourierId?: string | null;
  assignedBy: string;
  assignedAt: string;
};

/** Active deliveries and breakdown by status for a courier (API v1.1 section 5.3). */
export type CourierWorkload = {
  courierId: string;
  fullName: string;
  isAvailable: boolean;
  activeDeliveries: number;
  byStatus: Record<string, number>;
};

/** A parcel in the dispatcher/admin parcel search table (API v1.1 section 5.4). */
export type AdminParcelItem = {
  id: string;
  trackingNumber: string;
  status: string;
  pickupCity: string;
  dropoffCity: string;
  courierId?: string | null;
  merchantId?: string | null;
  customerId?: string | null;
  total: number;
  createdAt: string;
};

/** Search filters and pagination for GET /admin/parcels. */
export type AdminParcelsQuery = {
  status?: string;
  courierId?: string;
  merchantId?: string;
  customerId?: string;
  trackingNumber?: string;
  from?: string;
  to?: string;
  page?: number;
  pageSize?: number;
  sort?: string;
};

/** Courier item returned by GET /couriers (API v1.1 section 2.15). */
export type AvailableCourier = {
  id: string;
  fullName: string;
  phone?: string;
  vehicleType?: string | null;
  homeZoneId?: string | null;
  isAvailable: boolean;
};

export type CourierQuery = {
  isAvailable?: boolean;
  zoneId?: string;
  page?: number;
  pageSize?: number;
};

function toQueryString(params: Record<string, unknown>): string {
  const searchParams = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== null && value !== '') {
      searchParams.set(key, String(value));
    }
  }
  const str = searchParams.toString();
  return str ? `?${str}` : '';
}

/** Lists unassigned parcels with filtering and pagination (Section 5.1). */
export function listUnassignedParcels(
  query: UnassignedQuery = {},
  signal?: AbortSignal,
): Promise<{ data: UnassignedParcel[]; meta: ApiMeta }> {
  return apiWithMeta<UnassignedParcel[]>(
    `/dispatch/unassigned${toQueryString(query)}`,
    { signal },
  );
}

/** Assigns or reassigns a parcel to a courier (Section 5.2). */
export function assignCourier(
  parcelId: string,
  input: AssignCourierInput,
  signal?: AbortSignal,
): Promise<AssignCourierResult> {
  return api<AssignCourierResult>(
    `/dispatch/parcels/${encodeURIComponent(parcelId)}/assignment`,
    {
      method: 'PUT',
      body: input,
      signal,
    },
  );
}

/** Returns the active delivery workload across all active couriers (Section 5.3). */
export function getCourierWorkload(
  signal?: AbortSignal,
): Promise<CourierWorkload[]> {
  return api<CourierWorkload[]>('/dispatch/workload', { signal });
}

/** Searches all parcels across all statuses and couriers (Section 5.4). */
export function searchAllParcels(
  query: AdminParcelsQuery = {},
  signal?: AbortSignal,
): Promise<{ data: AdminParcelItem[]; meta: ApiMeta }> {
  return apiWithMeta<AdminParcelItem[]>(
    `/admin/parcels${toQueryString(query)}`,
    { signal },
  );
}

/** Lists couriers, optionally filtered by availability or zone (Section 2.15). */
export function listCouriers(
  query: CourierQuery = {},
  signal?: AbortSignal,
): Promise<{ data: AvailableCourier[]; meta: ApiMeta }> {
  return apiWithMeta<AvailableCourier[]>(`/couriers${toQueryString(query)}`, {
    signal,
  });
}

/** Lists active delivery zones for pickup dropdowns (Section 3.2). */
export function listZones(signal?: AbortSignal): Promise<Zone[]> {
  return api<Zone[]>('/zones', { signal });
}

