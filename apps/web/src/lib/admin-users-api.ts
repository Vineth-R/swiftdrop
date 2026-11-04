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

/** The largest page the API serves (API v1.1 section 1.5). */
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
