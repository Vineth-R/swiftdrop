import { api, apiWithMeta, type ApiMeta } from './api';

/** The calls and types of the pricing service (API v1.1 section 3). */

export const sizeCategories = [
  'SMALL',
  'MEDIUM',
  'LARGE',
  'OVERSIZED',
] as const;
export type SizeCategory = (typeof sizeCategories)[number];

/** A zone in the public list, which holds active zones only (section 3.2). */
export type Zone = {
  id: string;
  code: string;
  name: string;
  districts: string[];
};

/** A zone as the admin calls return it (sections 3.3 and 3.4). */
export type AdminZone = Zone & { isActive: boolean };

export type EstimateInput = {
  originZoneId: string;
  destinationZoneId: string;
  weightKg: number;
  sizeCategory: SizeCategory;
};

export type Estimate = EstimateInput & {
  currency: string;
  baseFee: number;
  weightCharge: number;
  sizeSurcharge: number;
  total: number;
};

export type PricingRule = {
  id: string;
  originZoneId: string;
  destinationZoneId: string;
  baseFee: number;
  currency: string;
  isActive: boolean;
  updatedAt: string;
};

export type WeightBand = {
  id: string;
  minWeightKg: number;
  maxWeightKg: number;
  charge: number;
  isActive: boolean;
};

export type SizeSurcharge = { sizeCategory: SizeCategory; surcharge: number };

/** The page size the admin screens ask for. 100 is the API's maximum. */
export const PRICING_PAGE_SIZE = 20;
export const PRICING_MAX_PAGE_SIZE = 100;

type Page = {
  page?: number;
  pageSize?: number;
  /** The API orders by id by default, which reads as random on a screen. */
  sort?: 'createdAt:asc';
};

export type AdminZoneQuery = Page & { isActive?: boolean };

export type PricingRuleQuery = Page & {
  originZoneId?: string;
  destinationZoneId?: string;
  isActive?: boolean;
};

/** Leaves out what is not set. `isActive: false` is a filter, so it stays. */
function toQueryString(query: AdminZoneQuery | PricingRuleQuery): string {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value !== undefined && value !== '') params.set(key, String(value));
  }
  const text = params.toString();
  return text ? `?${text}` : '';
}

/** Section 3.1. Public. The server works out the fee again when a parcel is created. */
export function estimate(
  input: EstimateInput,
  signal?: AbortSignal,
): Promise<Estimate> {
  return api<Estimate>('/pricing/estimate', {
    method: 'POST',
    body: input,
    signal,
  });
}

/** Section 3.2. Public. Fills the pickup and destination dropdowns. */
export function getZones(signal?: AbortSignal): Promise<Zone[]> {
  return api<Zone[]>('/zones', { signal });
}

/** Section 3.13. Admin only. Unlike getZones, it includes inactive zones. */
export function listAdminZones(
  query: AdminZoneQuery = {},
  signal?: AbortSignal,
): Promise<{ data: AdminZone[]; meta: ApiMeta }> {
  return apiWithMeta<AdminZone[]>(`/admin/zones${toQueryString(query)}`, {
    signal,
  });
}

/** Section 3.3. Admin only. */
export function createZone(input: {
  code: string;
  name: string;
  districts?: string[];
}): Promise<AdminZone> {
  return api<AdminZone>('/admin/zones', { method: 'POST', body: input });
}

/** Section 3.4. Admin only. Send only what changed. */
export function updateZone(
  zoneId: string,
  changes: { name?: string; districts?: string[]; isActive?: boolean },
): Promise<AdminZone> {
  return api<AdminZone>(`/admin/zones/${encodeURIComponent(zoneId)}`, {
    method: 'PATCH',
    body: changes,
  });
}

/** Section 3.5. Admin only. */
export function listPricingRules(
  query: PricingRuleQuery = {},
  signal?: AbortSignal,
): Promise<{ data: PricingRule[]; meta: ApiMeta }> {
  return apiWithMeta<PricingRule[]>(
    `/admin/pricing/rules${toQueryString(query)}`,
    { signal },
  );
}

/** Section 3.6. Admin only. */
export function createPricingRule(input: {
  originZoneId: string;
  destinationZoneId: string;
  baseFee: number;
}): Promise<PricingRule> {
  return api<PricingRule>('/admin/pricing/rules', {
    method: 'POST',
    body: input,
  });
}

/** Section 3.7. Admin only. Never alters the fee stored on an existing parcel. */
export function updatePricingRule(
  ruleId: string,
  changes: { baseFee?: number; isActive?: boolean },
): Promise<PricingRule> {
  return api<PricingRule>(
    `/admin/pricing/rules/${encodeURIComponent(ruleId)}`,
    { method: 'PATCH', body: changes },
  );
}

/** Section 3.8. Admin only. */
export function listWeightBands(
  query: Page = {},
  signal?: AbortSignal,
): Promise<{ data: WeightBand[]; meta: ApiMeta }> {
  return apiWithMeta<WeightBand[]>(
    `/admin/pricing/bands${toQueryString(query)}`,
    { signal },
  );
}

/** Section 3.9. Admin only. */
export function createWeightBand(input: {
  minWeightKg: number;
  maxWeightKg: number;
  charge: number;
}): Promise<WeightBand> {
  return api<WeightBand>('/admin/pricing/bands', {
    method: 'POST',
    body: input,
  });
}

/** Section 3.10. Admin only. */
export function updateWeightBand(
  bandId: string,
  changes: {
    minWeightKg?: number;
    maxWeightKg?: number;
    charge?: number;
    isActive?: boolean;
  },
): Promise<WeightBand> {
  return api<WeightBand>(`/admin/pricing/bands/${encodeURIComponent(bandId)}`, {
    method: 'PATCH',
    body: changes,
  });
}

/** Section 3.11. Admin only. There is one row per size, so no paging is needed. */
export function listSizeSurcharges(
  signal?: AbortSignal,
): Promise<SizeSurcharge[]> {
  return api<SizeSurcharge[]>('/admin/pricing/surcharges', { signal });
}

/** Section 3.12. Admin only. */
export function setSizeSurcharge(
  sizeCategory: SizeCategory,
  surcharge: number,
): Promise<SizeSurcharge> {
  return api<SizeSurcharge>(`/admin/pricing/surcharges/${sizeCategory}`, {
    method: 'PUT',
    body: { surcharge },
  });
}
