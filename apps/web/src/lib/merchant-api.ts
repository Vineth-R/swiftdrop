import { api, apiWithMeta } from './api';
import type { MerchantStatus } from '../components/ui/status-chip';
import type { ParcelStatus } from './tokens';

export type Merchant = {
  id: string;
  ownerUserId: string;
  businessName: string;
  registrationNumber?: string;
  businessAddress?: string;
  contactEmail: string;
  contactPhone: string;
  pickupAddress: string;
  pickupZoneId: string;
  status: MerchantStatus;
  approvedAt?: string;
  rejectionReason?: string;
  suspensionReason?: string;
  /**
   * Set by the administrator's list and detail only: a rejection or suspension
   * that has not yet reached Identity, so the account may still hold sessions.
   */
  identitySync?: 'PENDING';
  createdAt?: string;
  updatedAt?: string;
};
export type MerchantProfileInput = Pick<
  Merchant,
  'contactEmail' | 'contactPhone' | 'pickupAddress' | 'pickupZoneId'
>;
export type MerchantRegistration = {
  fullName: string;
  email: string;
  password: string;
  phone: string;
  business: {
    name: string;
    registrationNumber: string;
    businessAddress: string;
  } & MerchantProfileInput;
};
export type MerchantKey = {
  id: string;
  name: string;
  keyPrefix: string;
  status: 'ACTIVE' | 'REVOKED';
  createdAt: string;
  lastUsedAt?: string | null;
};
export type CreatedMerchantKey = Pick<
  MerchantKey,
  'id' | 'name' | 'keyPrefix'
> & { apiKey: string };
export type MerchantOrder = {
  orderId: string;
  externalOrderId: string;
  trackingNumber: string;
  status: ParcelStatus;
  recipientName: string;
  dropoffCity: string;
  total: number;
  currency: string;
  createdAt: string;
  updatedAt: string;
};
export type OrderQuery = {
  status?: string;
  from?: string;
  to?: string;
  externalOrderId?: string;
  page: number;
  pageSize?: number;
  sort?: string;
};

export function getMerchant(signal?: AbortSignal) {
  return api<Merchant>('/merchants/me', { signal });
}
export function updateMerchant(body: MerchantProfileInput) {
  return api<Merchant>('/merchants/me', { method: 'PATCH', body });
}
export function registerMerchant(
  body: MerchantRegistration,
  idempotencyKey: string,
) {
  return api<{ merchantId: string; userId: string; status: MerchantStatus }>(
    '/merchants/register',
    {
      method: 'POST',
      body,
      headers: { 'Idempotency-Key': idempotencyKey },
      renewSession: false,
    },
  );
}

export type AdminMerchantQuery = {
  status?: string;
  q?: string;
  page: number;
  pageSize?: number;
  sort?: string;
};
export function listAdminMerchants(
  query: AdminMerchantQuery,
  signal?: AbortSignal,
) {
  const params = new URLSearchParams({
    page: String(query.page),
    pageSize: String(query.pageSize ?? 20),
  });
  if (query.status) params.set('status', query.status);
  if (query.q) params.set('q', query.q);
  if (query.sort) params.set('sort', query.sort);
  return apiWithMeta<Merchant[]>(`/admin/merchants?${params}`, { signal });
}
export function getAdminMerchant(id: string, signal?: AbortSignal) {
  return api<Merchant>(`/admin/merchants/${encodeURIComponent(id)}`, {
    signal,
  });
}
export type MerchantDecision = {
  id: string;
  status: MerchantStatus;
  identitySync: 'APPLIED' | 'PENDING';
  alreadyDone: boolean;
};
export function approveMerchant(id: string) {
  return api<MerchantDecision>(
    `/admin/merchants/${encodeURIComponent(id)}/approve`,
    { method: 'POST' },
  );
}
export function rejectMerchant(id: string, reason?: string) {
  return api<MerchantDecision>(
    `/admin/merchants/${encodeURIComponent(id)}/reject`,
    {
      method: 'POST',
      body: reason?.trim() ? { reason: reason.trim() } : {},
    },
  );
}
export function suspendMerchant(id: string, reason?: string) {
  return api<MerchantDecision>(
    `/admin/merchants/${encodeURIComponent(id)}/suspend`,
    {
      method: 'POST',
      body: reason?.trim() ? { reason: reason.trim() } : {},
    },
  );
}
export function listMerchantKeys(signal?: AbortSignal) {
  return api<MerchantKey[]>('/merchants/me/keys', { signal });
}
export function createMerchantKey(name: string) {
  return api<CreatedMerchantKey>('/merchants/me/keys', {
    method: 'POST',
    body: { name },
  });
}
export function revokeMerchantKey(id: string) {
  return api(`/merchants/me/keys/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  });
}
// The list fields are defined in the M8 brief. Order-detail fields are not yet
// specified in OpenAPI; keep the detail UI on these agreed summary fields.
export function listMerchantOrders(query: OrderQuery, signal?: AbortSignal) {
  const params = new URLSearchParams({
    page: String(query.page),
    pageSize: String(query.pageSize ?? 20),
  });
  for (const [key, value] of Object.entries(query)) {
    if (value && key !== 'page' && key !== 'pageSize') {
      params.set(key, String(value));
    }
  }
  return apiWithMeta<MerchantOrder[]>(`/merchant/orders?${params}`, { signal });
}

export type MerchantOrderOverview = {
  total: number;
  inProgress: number;
  delivered: number;
  failed: number;
  recent: MerchantOrder[];
};

/**
 * The dashboard needs four totals and a short recent list. The API has no
 * separate summary endpoint, so this reads the list pagination totals instead
 * of inventing a route or hardcoding numbers.
 */
export async function getMerchantOrderOverview(
  signal?: AbortSignal,
): Promise<MerchantOrderOverview> {
  const [all, delivered, failed, recent] = await Promise.all([
    listMerchantOrders({ page: 1, pageSize: 1 }, signal),
    listMerchantOrders({ page: 1, pageSize: 1, status: 'DELIVERED' }, signal),
    listMerchantOrders({ page: 1, pageSize: 1, status: 'FAILED' }, signal),
    listMerchantOrders(
      { page: 1, pageSize: 5, sort: 'createdAt:desc' },
      signal,
    ),
  ]);
  const total = all.meta.pagination?.totalItems ?? all.data.length;
  const deliveredCount = delivered.meta.pagination?.totalItems ?? 0;
  const failedCount = failed.meta.pagination?.totalItems ?? 0;
  return {
    total,
    delivered: deliveredCount,
    failed: failedCount,
    inProgress: Math.max(0, total - deliveredCount - failedCount),
    recent: recent.data,
  };
}

/** API documentation v1.1, section 8.18; timeline follows section 4.6. */
export type MerchantOrderDetail = MerchantOrder & {
  weightKg: number;
  sizeCategory: string;
  description?: string | null;
  fee: {
    currency: string;
    baseFee: number;
    weightCharge: number;
    sizeSurcharge: number;
    total: number;
  };
  timeline: { status: ParcelStatus; at: string }[];
};
export function getMerchantOrder(id: string, signal?: AbortSignal) {
  return api<MerchantOrderDetail>(
    `/merchant/orders/${encodeURIComponent(id)}`,
    { signal },
  );
}
