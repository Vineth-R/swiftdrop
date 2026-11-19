import { apiWithMeta } from './api';

export const auditLogSources = ['identity', 'merchant', 'pricing'] as const;
export type AuditLogSource = (typeof auditLogSources)[number];

export type AuditLogEntry = {
  id: string;
  actorUserId: string;
  action: string;
  entityType: string;
  entityId: string | null;
  details: Record<string, unknown>;
  createdAt: string;
};

export type AuditLogPage = {
  items: AuditLogEntry[];
  pagination: {
    page: number;
    pageSize: number;
    totalItems: number;
    totalPages: number;
  };
};

export type AuditLogFilters = {
  page?: number;
  pageSize?: number;
  actorUserId?: string;
  action?: string;
  entityType?: string;
  entityId?: string;
  from?: string;
  to?: string;
  sort?: 'createdAt:asc' | 'createdAt:desc';
  signal?: AbortSignal;
};

/** Read one ADMIN audit source. The API redacts credential-like detail keys. */
export async function listAuditLogs(
  source: AuditLogSource,
  {
    page = 1,
    pageSize = 20,
    actorUserId,
    action,
    entityType,
    entityId,
    from,
    to,
    sort = 'createdAt:desc',
    signal,
  }: AuditLogFilters = {},
): Promise<AuditLogPage> {
  const query = new URLSearchParams({
    page: String(page),
    pageSize: String(pageSize),
    sort,
  });
  if (actorUserId) query.set('actorUserId', actorUserId);
  if (action) query.set('action', action);
  if (entityType) query.set('entityType', entityType);
  if (entityId) query.set('entityId', entityId);
  if (from) query.set('from', from);
  if (to) query.set('to', to);

  const { data, meta } = await apiWithMeta<AuditLogEntry[]>(
    `/admin/audit-logs/${source}?${query.toString()}`,
    { signal },
  );
  return {
    items: data,
    pagination: meta.pagination ?? {
      page,
      pageSize,
      totalItems: data.length,
      totalPages: data.length > 0 ? 1 : 0,
    },
  };
}
