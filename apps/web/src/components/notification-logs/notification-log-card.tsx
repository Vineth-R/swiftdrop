import { CircleCheck, CircleX, Clock, type LucideIcon } from 'lucide-react';

import { cn } from '../../lib/utils';
import { formatDateTime } from '../../lib/format';
import type {
  NotificationLog,
  NotificationLogStatus,
} from '../../lib/notification-logs-api';
import { Badge } from '../ui/badge';

const statusLooks: Record<
  NotificationLogStatus,
  { className: string; Icon: LucideIcon }
> = {
  SENT: {
    className: 'border-success-border bg-success-tint text-success',
    Icon: CircleCheck,
  },
  PENDING: {
    className: 'border-warning-border bg-warning-tint text-warning',
    Icon: Clock,
  },
  FAILED: {
    className: 'border-danger-border bg-danger-tint text-danger',
    Icon: CircleX,
  },
};

export function NotificationLogStatusBadge({
  status,
  className,
}: {
  status: NotificationLogStatus;
  className?: string;
}) {
  const { className: look, Icon } = statusLooks[status];
  return (
    <Badge
      variant="outline"
      className={cn('gap-2 rounded-chip text-caption', look, className)}
    >
      <Icon size={12} strokeWidth={1.5} aria-hidden="true" />
      {status}
    </Badge>
  );
}

/** The phone-sized view of one delivery attempt. */
export function NotificationLogCard({ log }: { log: NotificationLog }) {
  return (
    <article className="flex flex-col gap-3 rounded-card border border-line bg-surface p-4">
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 flex-col gap-1">
          <span className="truncate font-mono text-body-sm">
            {log.recipient}
          </span>
          <span className="text-caption text-text-muted">
            {log.channel} · {formatDateTime(log.createdAt)}
          </span>
        </div>
        <NotificationLogStatusBadge status={log.status} />
      </div>
      <div className="flex flex-col gap-1">
        <span className="text-body-sm">{log.subject ?? 'No subject'}</span>
        {log.errorMessage && (
          <span className="text-caption text-danger">{log.errorMessage}</span>
        )}
      </div>
      <div className="flex flex-wrap items-center justify-between gap-2 border-t border-line pt-3 text-caption text-text-muted">
        <span>Event {log.eventId.slice(0, 8)}</span>
        <span>
          {log.sentAt ? `Sent ${formatDateTime(log.sentAt)}` : 'Not sent'}
        </span>
      </div>
    </article>
  );
}
