import { api, apiWithMeta, type ApiMeta } from './api';
import type { Role } from './tokens';

/** A user as the admin list returns them (API v1.1 section 2.10). */
export type AdminUser = {
  id: string;
  email: string;
  fullName: string;
  /** Empty when the account has no phone number. */
  phone: string;
  role: Role;
  isActive: boolean;
  createdAt: string;
};

/** The query of the list (API v1.1 section 2.10). Leave out what is not set. */
export type AdminUserQuery = {
  role?: Role;
  isActive?: boolean;
  /** Searches the name and the email. */
  q?: string;
  /** "createdAt", "fullName" or "email", optionally followed by ":asc" or ":desc". */
  sort?: string;
  page?: number;
  pageSize?: number;
};

/**
 * The page size the screen asks for. 20 is the API's default page size and 100
 * its maximum (API v1.1 section 1.5).
 */
export const ADMIN_USERS_PAGE_SIZE = 20;

function toQueryString(query: AdminUserQuery): string {
  const params = new URLSearchParams();
  if (query.role) params.set('role', query.role);
  if (query.isActive !== undefined) {
    params.set('isActive', String(query.isActive));
  }
  if (query.q) params.set('q', query.q);
  if (query.sort) params.set('sort', query.sort);
  if (query.page) params.set('page', String(query.page));
  if (query.pageSize) params.set('pageSize', String(query.pageSize));
  const text = params.toString();
  return text ? `?${text}` : '';
}

/**
 * One page of users, newest first unless `sort` says otherwise. Admin only.
 * The collection path ends in a slash on purpose: the gateway forwards
 * /api/v1/admin/users/ and has no rule for the same path without it.
 */
export function listAdminUsers(
  query: AdminUserQuery,
  signal?: AbortSignal,
): Promise<{ data: AdminUser[]; meta: ApiMeta }> {
  return apiWithMeta<AdminUser[]>(`/admin/users/${toQueryString(query)}`, {
    signal,
  });
}

/**
 * Activates or deactivates a user (API v1.1 section 2.14). Deactivating signs
 * the user out of every device. An admin cannot deactivate themselves.
 */
export function setUserActive(
  userId: string,
  isActive: boolean,
): Promise<{ id: string; isActive: boolean }> {
  return api<{ id: string; isActive: boolean }>(
    `/admin/users/${encodeURIComponent(userId)}/status`,
    { method: 'PATCH', body: { isActive } },
  );
}

/** The roles an administrator can create an account for (API v1.1 section 2.11). */
export const createableRoles = ['COURIER', 'DISPATCHER', 'ADMIN'] as const;
export type CreateableRole = (typeof createableRoles)[number];

/** A courier's vehicle and home zone. Every part is optional. */
export type NewCourierProfile = {
  vehicleType?: string;
  vehicleNumber?: string;
  homeZoneId?: string;
};

/** The body of POST /admin/users (API v1.1 section 2.11). */
export type NewStaffUser = {
  email: string;
  password: string;
  fullName: string;
  /** E.164, for example +94771234567. */
  phone: string;
  role: CreateableRole;
  /** Required for COURIER and refused for every other role. */
  courierProfile?: NewCourierProfile;
};

/**
 * Creates a courier, dispatcher or administrator account (API v1.1 section
 * 2.11). Admin only. The new user is told by email that the account exists;
 * the email never carries the password.
 */
export function createAdminUser(input: NewStaffUser): Promise<AdminUser> {
  return api<AdminUser>('/admin/users/', { method: 'POST', body: input });
}

/** A courier's profile as the admin detail returns it (API v1.1 section 2.12). */
export type AdminCourierProfile = {
  vehicleType: string | null;
  vehicleNumber: string | null;
  homeZoneId: string | null;
  isAvailable: boolean;
};

/** One user in full (API v1.1 section 2.12), with `courierProfile` when they have one. */
export type AdminUserDetail = AdminUser & {
  courierProfile?: AdminCourierProfile;
  lastLoginAt: string | null;
  updatedAt: string;
};

/**
 * The roles an administrator can set on an existing user (API v1.1 section
 * 2.13). A merchant account keeps its role: it belongs to a merchant record.
 */
export const editableRoles = [
  'CUSTOMER',
  'COURIER',
  'DISPATCHER',
  'ADMIN',
] as const;
export type EditableRole = (typeof editableRoles)[number];

/**
 * What PATCH /admin/users/{userId} accepts: any subset, and at least one field.
 * A courier's vehicle and home zone can be changed but not cleared.
 */
export type UserChange = {
  fullName?: string;
  /** E.164, for example +94771234567. */
  phone?: string;
  role?: EditableRole;
  /** Required when the role becomes COURIER, and refused for any other role. */
  courierProfile?: {
    vehicleType?: string;
    vehicleNumber?: string;
    homeZoneId?: string;
    isAvailable?: boolean;
  };
};

/** One user in full. Admin only. A missing or malformed id is a 404. */
export function getAdminUser(
  userId: string,
  signal?: AbortSignal,
): Promise<AdminUserDetail> {
  return api<AdminUserDetail>(`/admin/users/${encodeURIComponent(userId)}`, {
    signal,
  });
}

/**
 * Changes a user's name, phone, role or courier details (API v1.1 section
 * 2.13). Admin only. An administrator cannot change their own role.
 */
export function updateAdminUser(
  userId: string,
  change: UserChange,
): Promise<AdminUserDetail> {
  return api<AdminUserDetail>(`/admin/users/${encodeURIComponent(userId)}`, {
    method: 'PATCH',
    body: change,
  });
}
