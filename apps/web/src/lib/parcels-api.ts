import { api, apiBlob, apiWithMeta } from './api';
import type { SizeCategory } from './pricing-api';
import type { ParcelStatus } from './tokens';

export type ParcelContact = {
  name: string;
  phone: string;
  address: string;
  city: string;
  zoneId: string;
};

export type ParcelFee = {
  currency: string;
  baseFee: number;
  weightCharge: number;
  sizeSurcharge: number;
  total: number;
};

export type NewParcel = {
  sender: ParcelContact;
  recipient: ParcelContact;
  weightKg: number;
  sizeCategory: SizeCategory;
  description?: string;
  expectedFee: number;
};

export type Parcel = Omit<NewParcel, 'expectedFee' | 'description'> & {
  description: string | null;
  id: string;
  trackingNumber: string;
  status: ParcelStatus;
  fee: ParcelFee;
  courier: { id?: string; fullName: string; phone: string } | null;
  failureReason: string | null;
  createdAt: string;
  updatedAt: string;
  deliveredAt: string | null;
};

export type CreatedParcel = Pick<
  Parcel,
  | 'id'
  | 'trackingNumber'
  | 'status'
  | 'weightKg'
  | 'sizeCategory'
  | 'description'
  | 'fee'
  | 'createdAt'
> & {
  sender: Pick<ParcelContact, 'name' | 'city'>;
  recipient: Pick<ParcelContact, 'name' | 'city'>;
};

export type ParcelSummary = {
  id: string;
  trackingNumber: string;
  status: ParcelStatus;
  recipientName: string;
  dropoffCity: string;
  total: number;
  currency: string;
  createdAt: string;
};

export type ParcelHistory = {
  previousStatus: ParcelStatus | null;
  status: ParcelStatus;
  note: string | null;
  failureReason: string | null;
  actorRole: string | null;
  changedAt: string;
};

export type ProofPhotoTarget = {
  url: string;
  expiresAt: string;
  mimeType: 'image/jpeg' | 'image/png';
  stream: boolean;
};

export type ParcelQuery = {
  status?: ParcelStatus | '';
  from?: string;
  to?: string;
  page?: number;
  pageSize?: number;
  sort?: 'createdAt:desc' | 'createdAt:asc';
};

export function createParcel(input: NewParcel, idempotencyKey: string) {
  return api<CreatedParcel>('/parcels', {
    method: 'POST',
    body: input,
    headers: { 'Idempotency-Key': idempotencyKey },
  });
}

export function listParcels(query: ParcelQuery, signal?: AbortSignal) {
  const params = new URLSearchParams();
  for (const [name, value] of Object.entries(query)) {
    if (value !== '' && value !== undefined) params.set(name, String(value));
  }
  return apiWithMeta<ParcelSummary[]>(`/parcels?${params}`, { signal });
}

export function getParcel(id: string, signal?: AbortSignal) {
  return api<Parcel>(`/parcels/${encodeURIComponent(id)}`, { signal });
}

export function getParcelHistory(id: string, signal?: AbortSignal) {
  return api<ParcelHistory[]>(`/parcels/${encodeURIComponent(id)}/history`, {
    signal,
  });
}

export function getParcelReceipt(id: string, signal?: AbortSignal) {
  return apiBlob(`/parcels/${encodeURIComponent(id)}/receipt`, { signal });
}

/** Returns a short-lived proof photo target; no permanent URL is exposed. */
export function getParcelProofPhoto(id: string, signal?: AbortSignal) {
  return api<ProofPhotoTarget>(
    `/parcels/${encodeURIComponent(id)}/proof/photo`,
    { signal },
  );
}
