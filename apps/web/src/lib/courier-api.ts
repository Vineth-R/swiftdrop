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

export type CourierHistory = {
  previousStatus: string | null;
  status: string;
  note: string | null;
  failureReason: string | null;
  actorRole: string | null;
  changedAt: string;
};
export type CourierProof = {
  recipientName: string | null;
  note: string | null;
  hasPhoto: boolean;
  createdAt: string;
} | null;
export type CourierDeliveryDetail = CourierDelivery & {
  description: string | null;
  sizeCategory: string;
  senderName: string;
  senderPhone: string;
  history: CourierHistory[];
  proof: CourierProof;
};
export type CourierStatusUpdate = {
  status: ParcelStatus;
  recipientName?: string;
  failureReason?: string;
  note?: string;
};

export function getCourierDelivery(id: string, signal?: AbortSignal) {
  return api<CourierDeliveryDetail>(
    `/courier/deliveries/${encodeURIComponent(id)}`,
    { signal },
  );
}

export function updateCourierDeliveryStatus(
  id: string,
  body: CourierStatusUpdate,
) {
  return api<{
    status: ParcelStatus;
    previousStatus: ParcelStatus;
    updatedAt: string;
  }>(`/courier/deliveries/${encodeURIComponent(id)}/status`, {
    method: 'POST',
    body,
  });
}

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
