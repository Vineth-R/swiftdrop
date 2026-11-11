import { ApiError, api, apiWithMeta, type ApiMeta } from './api';
import type { ProofPhotoTarget } from './parcels-api';
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

export type CourierHistoryEntry = {
  previousStatus: ParcelStatus | null;
  status: ParcelStatus;
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
};

export type CourierDeliveryDetail = CourierDelivery & {
  description: string | null;
  sizeCategory: 'SMALL' | 'MEDIUM' | 'LARGE' | 'OVERSIZED';
  senderName: string;
  senderPhone: string;
  history: CourierHistoryEntry[];
  proof: CourierProof | null;
};

/** A status a courier may submit. CREATED is not a valid target. */
export type CourierTargetStatus =
  'PICKED_UP' | 'IN_TRANSIT' | 'OUT_FOR_DELIVERY' | 'DELIVERED' | 'FAILED';

export type CourierStatusUpdate = {
  status: CourierTargetStatus;
  recipientName?: string;
  failureReason?: string;
  note?: string;
};

export type CourierStatusResult = {
  parcelId: string;
  trackingNumber: string;
  previousStatus: ParcelStatus;
  status: ParcelStatus;
  updatedAt: string;
};

export type CourierProofUpload = {
  photo?: File;
  recipientName?: string;
  note?: string;
};

export type CourierProofResult = CourierProof & {
  id: string;
  parcelId: string;
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

/** Returns a short-lived proof photo target for an assigned delivery. */
export function getCourierProofPhoto(parcelId: string, signal?: AbortSignal) {
  return api<ProofPhotoTarget>(
    `/parcels/${encodeURIComponent(parcelId)}/proof/photo`,
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

/** Stores proof for the authenticated courier's currently assigned parcel. */
export function uploadCourierDeliveryProof(
  parcelId: string,
  proof: CourierProofUpload,
  signal?: AbortSignal,
) {
  const form = new FormData();
  if (proof.photo) form.append('photo', proof.photo);
  if (proof.recipientName) form.append('recipientName', proof.recipientName);
  if (proof.note) form.append('note', proof.note);
  return api<CourierProofResult>(
    `/courier/deliveries/${encodeURIComponent(parcelId)}/proof`,
    { method: 'POST', body: form, signal },
  );
}
