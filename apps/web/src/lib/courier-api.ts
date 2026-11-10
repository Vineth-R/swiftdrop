import { ApiError, api, apiWithMeta, type ApiMeta } from './api';
import type { ParcelStatus } from './tokens';

export type CompletedDeliveryStatus = 'DELIVERED' | 'FAILED';

/** API v1.1 §6.1. Completion times and failure reasons are not list fields. */
export type CourierDelivery = {
  id: string;
  trackingNumber: string;
  status: ParcelStatus;
  pickupAddress: string;
  dropoffAddress?: string | null;
  dropoffCity?: string | null;
  recipientName?: string | null;
  recipientPhone?: string | null;
  weightKg: number;
  assignedAt: string;
};

export type CourierDeliveryPage = {
  items: CourierDelivery[];
  pagination: NonNullable<ApiMeta['pagination']>;
};

export async function listCourierDeliveries({
  date,
  status,
  page,
  signal,
}: {
  /** Calendar date (YYYY-MM-DD) in Asia/Colombo, not a UTC timestamp. */
  date: string;
  /** Omit to list every delivery for the day, including active parcels. */
  status?: CompletedDeliveryStatus;
  page: number;
  signal?: AbortSignal;
}): Promise<CourierDeliveryPage> {
  const query = new URLSearchParams({
    date,
    page: String(page),
    pageSize: '20',
  });
  if (status) query.set('status', status);
  const { data, meta } = await apiWithMeta<CourierDelivery[]>(
    `/courier/deliveries?${query}`,
    { signal },
  );
  const pagination = meta.pagination;

  // Missing metadata is a contract error, never an invented single-page result.
  if (
    !Array.isArray(data) ||
    !pagination ||
    !Number.isInteger(pagination.page) ||
    pagination.page !== page ||
    !Number.isInteger(pagination.pageSize) ||
    pagination.pageSize < 1 ||
    !Number.isInteger(pagination.totalItems) ||
    pagination.totalItems < 0 ||
    !Number.isInteger(pagination.totalPages) ||
    pagination.totalPages < 0
  ) {
    throw new ApiError({
      code: 'INTERNAL',
      status: 200,
      requestId: meta.requestId,
    });
  }

  return { items: data, pagination };
}

/** Reads the authenticated courier's assignment-scoped delivery detail. */
export function getCourierDelivery(parcelId: string, signal?: AbortSignal) {
  return api<CourierDeliveryDetail>(
    `/courier/deliveries/${encodeURIComponent(parcelId)}`,
    { signal },
  );
}

/** Saves one documented transition for the authenticated courier. */
export function updateCourierDeliveryStatus(
  parcelId: string,
  update: CourierStatusUpdate,
  signal?: AbortSignal,
) {
  return api<CourierStatusResult>(
    `/courier/deliveries/${encodeURIComponent(parcelId)}/status`,
    {
      method: 'POST',
      body: update,
      signal,
    },
  );
}
