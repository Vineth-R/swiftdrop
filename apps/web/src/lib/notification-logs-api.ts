import { apiWithMeta } from './api';

export const notificationLogStatuses = ['PENDING', 'SENT', 'FAILED'] as const;
export type NotificationLogStatus = (typeof notificationLogStatuses)[number];

export type NotificationLog = {
  id: string;
  eventId: string;
  parcelId: string;
  channel: 'EMAIL' | 'SMS';
  /** Already masked by the API. Never a full address or phone number. */
  recipient: string;
  subject: string | null;
  status: NotificationLogStatus;
  errorMessage: string | null;
  createdAt: string;
  sentAt: string | null;
};

export type NotificationLogPagination = {
  page: number;
  pageSize: number;
  totalItems: number;
  totalPages: number;
};

export type NotificationLogPage = {
  items: NotificationLog[];
  pagination: NotificationLogPagination;
};

type ListOptions = {
  page?: number;
  pageSize?: number;
  status?: NotificationLogStatus;
  signal?: AbortSignal;
};

/** The ADMIN notification-log read API. Recipients are masked by the service. */
export async function listNotificationLogs({
  page = 1,
  pageSize = 20,
  status,
  signal,
}: ListOptions = {}): Promise<NotificationLogPage> {
  const query = new URLSearchParams({
    page: String(page),
    pageSize: String(pageSize),
  });
  if (status) query.set('status', status);

  const { data, meta } = await apiWithMeta<NotificationLog[]>(
    `/admin/notification-logs?${query.toString()}`,
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
