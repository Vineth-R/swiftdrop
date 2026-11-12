'use client';

import { BellOff, RefreshCw } from 'lucide-react';
import { useEffect, useState } from 'react';

import { ApiError } from '../../lib/api';
import {
  listNotificationLogs,
  notificationLogStatuses,
  type NotificationLogPage,
  type NotificationLogStatus,
} from '../../lib/notification-logs-api';
import { PageHeader } from '../layout/page-header';
import { Section } from '../layout/section';
import { Alert } from '../ui/alert';
import { Button } from '../ui/button';
import { Card, CardContent } from '../ui/card';
import { EmptyState } from '../ui/empty-state';
import {
  Pagination,
  PaginationContent,
  PaginationItem,
  PaginationNext,
  PaginationPrevious,
} from '../ui/pagination';
import {
  Select as SelectMenu,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../ui/select-menu';
import { Skeleton, SkeletonGroup, SkeletonText } from '../ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '../ui/table';
import { formatDateTime } from '../../lib/format';
import {
  NotificationLogCard,
  NotificationLogStatusBadge,
} from './notification-log-card';

const PAGE_SIZE = 20;

type StatusFilter = 'ALL' | NotificationLogStatus;

const emptyPage: NotificationLogPage = {
  items: [],
  pagination: {
    page: 1,
    pageSize: PAGE_SIZE,
    totalItems: 0,
    totalPages: 0,
  },
};

export function NotificationLogsView() {
  const [status, setStatus] = useState<StatusFilter>('ALL');
  const [page, setPage] = useState(1);
  const [result, setResult] = useState<NotificationLogPage>(emptyPage);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<ApiError | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    setIsLoading(true);
    setError(null);

    listNotificationLogs({
      page,
      pageSize: PAGE_SIZE,
      status: status === 'ALL' ? undefined : status,
      signal: controller.signal,
    })
      .then((next) => {
        if (!controller.signal.aborted) setResult(next);
      })
      .catch((caught: unknown) => {
        if (
          controller.signal.aborted ||
          (caught instanceof DOMException && caught.name === 'AbortError')
        ) {
          return;
        }
        setError(
          caught instanceof ApiError
            ? caught
            : new ApiError({ code: 'INTERNAL', status: 0 }),
        );
      })
      .finally(() => {
        if (!controller.signal.aborted) setIsLoading(false);
      });

    return () => controller.abort();
  }, [page, status, reloadKey]);

  function refresh() {
    setReloadKey((key) => key + 1);
  }

  function chooseStatus(next: string) {
    setStatus(next as StatusFilter);
    setPage(1);
  }

  const hasLogs = result.items.length > 0;
  const showEmpty = !isLoading && !error && !hasLogs;

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Notification logs"
        description="Delivery attempts recorded by the notification worker. Recipients are masked."
        actions={
          <Button
            variant="secondary"
            size="sm"
            isLoading={isLoading}
            onClick={refresh}
            iconLeading={
              <RefreshCw size={16} strokeWidth={1.5} aria-hidden="true" />
            }
          >
            Refresh
          </Button>
        }
      />

      <Section
        title="Delivery attempts"
        description="Newest first. Recipients are masked; failed attempts show the sanitized error."
        actions={
          <SelectMenu value={status} onValueChange={chooseStatus}>
            <SelectTrigger
              aria-label="Filter by delivery status"
              className="w-full sm:w-48"
            >
              <SelectValue placeholder="All statuses" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="ALL">All statuses</SelectItem>
              {notificationLogStatuses.map((value) => (
                <SelectItem key={value} value={value}>
                  {value}
                </SelectItem>
              ))}
            </SelectContent>
          </SelectMenu>
        }
      >
        <Card className="gap-0 overflow-hidden py-0">
          <CardContent className="p-0">
            {isLoading ? (
              <SkeletonGroup
                label="Loading notification logs"
                className="flex flex-col gap-5 p-4"
              >
                <SkeletonText lines={3} />
                <div className="flex flex-col gap-3">
                  {Array.from({ length: 4 }, (_, index) => (
                    <Skeleton key={index} className="h-12 w-full" />
                  ))}
                </div>
              </SkeletonGroup>
            ) : error ? (
              <div className="p-4">
                <Alert
                  tone="danger"
                  title={
                    error.code === 'FORBIDDEN'
                      ? 'You do not have access to notification logs'
                      : 'Notification logs could not be loaded'
                  }
                  actions={
                    <Button variant="secondary" size="sm" onClick={refresh}>
                      Try again
                    </Button>
                  }
                >
                  {error.isServerFault && error.requestId ? (
                    <span className="select-all font-mono text-caption text-text-faint">
                      Reference {error.requestId}
                    </span>
                  ) : error.code === 'FORBIDDEN' ? (
                    'This view is limited to administrators.'
                  ) : (
                    'The notification worker did not answer. Try again in a moment.'
                  )}
                </Alert>
              </div>
            ) : showEmpty ? (
              <div className="p-4">
                <EmptyState
                  title={
                    status === 'ALL'
                      ? 'No notification logs yet'
                      : `No ${status.toLowerCase()} logs`
                  }
                  icon={BellOff}
                  action={
                    <Button variant="secondary" size="sm" onClick={refresh}>
                      Refresh
                    </Button>
                  }
                >
                  Delivery attempts appear here after the notification worker
                  processes an event.
                </EmptyState>
              </div>
            ) : (
              <>
                <div className="hidden md:block">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead className="pl-4 text-overline uppercase text-text-faint">
                          Created
                        </TableHead>
                        <TableHead className="text-overline uppercase text-text-faint">
                          Type
                        </TableHead>
                        <TableHead className="text-overline uppercase text-text-faint">
                          Recipient
                        </TableHead>
                        <TableHead className="text-overline uppercase text-text-faint">
                          Subject
                        </TableHead>
                        <TableHead className="text-overline uppercase text-text-faint">
                          Status
                        </TableHead>
                        <TableHead className="pr-4 text-overline uppercase text-text-faint">
                          Sent
                        </TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {result.items.map((log) => (
                        <TableRow key={log.id}>
                          <TableCell className="pl-4 text-body-sm text-text-muted">
                            {formatDateTime(log.createdAt)}
                          </TableCell>
                          <TableCell className="text-body-sm text-text-muted">
                            {log.channel}
                          </TableCell>
                          <TableCell>
                            <div className="flex flex-col gap-1">
                              <span className="font-mono text-body-sm">
                                {log.recipient}
                              </span>
                            </div>
                          </TableCell>
                          <TableCell className="max-w-xs whitespace-normal">
                            <div className="flex flex-col gap-1">
                              <span className="text-body-sm">
                                {log.subject ?? 'No subject'}
                              </span>
                              {log.errorMessage && (
                                <span className="text-caption text-danger">
                                  {log.errorMessage}
                                </span>
                              )}
                            </div>
                          </TableCell>
                          <TableCell>
                            <NotificationLogStatusBadge status={log.status} />
                          </TableCell>
                          <TableCell className="pr-4 text-body-sm text-text-muted">
                            {log.sentAt ? formatDateTime(log.sentAt) : '—'}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>

                <ul className="flex flex-col gap-3 p-4 md:hidden">
                  {result.items.map((log) => (
                    <li key={log.id}>
                      <NotificationLogCard log={log} />
                    </li>
                  ))}
                </ul>
              </>
            )}
          </CardContent>
        </Card>
      </Section>

      {result.pagination.totalPages > 1 && (
        <Pagination className="justify-between gap-3">
          <p className="text-caption text-text-muted">
            {result.pagination.totalItems} logs · page {result.pagination.page}{' '}
            of {result.pagination.totalPages}
          </p>
          <PaginationContent>
            <PaginationItem>
              <PaginationPrevious
                onClick={() => setPage((current) => Math.max(1, current - 1))}
                disabled={page <= 1 || isLoading}
              />
            </PaginationItem>
            <PaginationItem>
              <PaginationNext
                onClick={() =>
                  setPage((current) =>
                    Math.min(result.pagination.totalPages, current + 1),
                  )
                }
                disabled={page >= result.pagination.totalPages || isLoading}
              />
            </PaginationItem>
          </PaginationContent>
        </Pagination>
      )}
    </div>
  );
}
