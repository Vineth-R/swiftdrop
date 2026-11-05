'use client';

import { useEffect, useState } from 'react';

import { ApiError } from '../../lib/api';
import {
  listCourierDeliveries,
  type CompletedDeliveryStatus,
  type CourierDelivery,
  type CourierDeliveryPage,
} from '../../lib/courier-api';
import { useSession } from '../../lib/session';
import { useCountdown } from '../../lib/use-countdown';
import { RateLimitedAlert, ServerFaultAlert } from '../auth/form-alerts';
import { PageHeader } from '../layout/page-header';
import { RequireRole } from '../layout/require-role';
import { Section } from '../layout/section';
import { Alert } from '../ui/alert';
import { Button } from '../ui/button';
import { DataTable, type DataTableColumn } from '../ui/data-table';
import { EmptyState } from '../ui/empty-state';
import { TextField } from '../ui/field';
import {
  Pagination,
  PaginationContent,
  PaginationItem,
  PaginationNext,
  PaginationPrevious,
} from '../ui/pagination';
import { Select } from '../ui/select';
import { StatusChip } from '../ui/status-chip';

const statuses = [
  { value: 'DELIVERED', label: 'Delivered' },
  { value: 'FAILED', label: 'Failed' },
];

const columns: DataTableColumn<CourierDelivery>[] = [
  {
    key: 'trackingNumber',
    header: 'Tracking number',
    isMono: true,
    cell: (delivery) => (
      <span className="break-all text-body">{delivery.trackingNumber}</span>
    ),
  },
  {
    key: 'status',
    header: 'Status',
    cell: (delivery) => <StatusChip status={delivery.status} />,
  },
  {
    key: 'recipientName',
    header: 'Recipient',
    cell: (delivery) => (
      <span className="block break-all text-body">
        {delivery.recipientName?.trim() || 'Not provided'}
      </span>
    ),
  },
  {
    key: 'destination',
    header: 'Destination',
    cell: (delivery) => (
      <span className="block break-all text-body">
        {[delivery.dropoffAddress?.trim(), delivery.dropoffCity?.trim()]
          .filter(Boolean)
          .join(', ') || 'Not provided'}
      </span>
    ),
  },
];

type Filters = { date: string; status: CompletedDeliveryStatus; page: number };
type LoadState = { key: string } & (
  | { status: 'loading' }
  | { status: 'ready'; result: CourierDeliveryPage }
  | { status: 'error'; error: ApiError }
);

function todayInColombo(): string {
  const parts = new Intl.DateTimeFormat('en-GB', {
    timeZone: 'Asia/Colombo',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).formatToParts(new Date());
  const part = (name: string) =>
    parts.find((item) => item.type === name)?.value;
  return `${part('year')}-${part('month')}-${part('day')}`;
}

export function HistoryView() {
  return (
    <RequireRole roles={['COURIER']} audience="couriers">
      <HistoryContent />
    </RequireRole>
  );
}

function HistoryContent() {
  const { session, recheck } = useSession();
  const userId = session.status === 'authenticated' ? session.user.id : '';
  const [filters, setFilters] = useState<Filters>({
    date: '',
    status: 'DELIVERED',
    page: 1,
  });
  const [hasDateBlurred, setHasDateBlurred] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const [state, setState] = useState<LoadState>({ key: '', status: 'loading' });
  const { date, status, page } = filters;
  const requestKey = JSON.stringify([userId, date, status, page, attempt]);
  // Hide the previous result immediately, even before effect cleanup runs.
  const current = state.key === requestKey ? state : null;
  const error = current?.status === 'error' ? current.error : null;
  const result = current?.status === 'ready' ? current.result : null;
  const isLoading = current === null || current.status === 'loading';
  const remaining = useCountdown(
    error?.code === 'RATE_LIMITED' ? (error.retryAfter ?? null) : null,
  );
  const hasNoPermission = error?.code === 'FORBIDDEN';
  const isBlocked = hasNoPermission || remaining > 0;
  const showPagination =
    Boolean(date) &&
    !error &&
    (isLoading || (result?.pagination.totalPages ?? 0) > 1);

  useEffect(() => {
    // Resolve the browser's current day after hydration, always in Colombo.
    setFilters((previous) => ({ ...previous, date: todayInColombo() }));
  }, []);

  useEffect(() => {
    if (!date || !userId) return;
    const controller = new AbortController();
    setState({ key: requestKey, status: 'loading' });

    listCourierDeliveries({ date, status, page, signal: controller.signal })
      .then((next) => {
        if (!controller.signal.aborted)
          setState({ key: requestKey, status: 'ready', result: next });
      })
      .catch((caught: unknown) => {
        if (controller.signal.aborted) return;
        const next =
          caught instanceof ApiError
            ? caught
            : new ApiError({ code: 'INTERNAL', status: 0 });
        if (next.code === 'ACCOUNT_DEACTIVATED') recheck();
        setState({ key: requestKey, status: 'error', error: next });
      });

    return () => controller.abort();
  }, [date, status, page, userId, attempt, requestKey, recheck]);

  return (
    <>
      <PageHeader
        title="History"
        description="Delivered and failed parcels for a selected day."
        actions={
          !hasNoPermission && (
            <Button
              variant="primary"
              size="lg"
              isLoading={Boolean(date) && isLoading}
              isDisabled={!date || isBlocked}
              onClick={() => setAttempt((previous) => previous + 1)}
            >
              {isLoading && date
                ? 'Loading history'
                : error
                  ? 'Try again'
                  : 'Refresh'}
            </Button>
          )
        }
      />

      <Section
        title="Filter history"
        description="Dates use Sri Lanka time (Asia/Colombo)."
      >
        {/* Shared lg fields shrink on desktop; couriers keep a 48px minimum. */}
        <div className="grid min-w-0 gap-4 md:grid-cols-2 [&_input]:min-h-12 [&_input:focus-visible]:outline-solid [&_select]:min-h-12">
          <TextField
            id="courier-history-date"
            label="Delivery date"
            type="date"
            controlSize="lg"
            value={date}
            disabled={isBlocked}
            isRequired
            error={
              hasDateBlurred && !date ? 'Choose a delivery date.' : undefined
            }
            onBlur={() => setHasDateBlurred(true)}
            onChange={(event) => {
              const next = event.target.value;
              setFilters((previous) => ({ ...previous, date: next, page: 1 }));
            }}
          />
          <Select
            id="courier-history-status"
            label="Delivery status"
            controlSize="lg"
            options={statuses}
            value={status}
            disabled={isBlocked}
            onChange={(event) => {
              const next = event.target.value;
              if (next === 'DELIVERED' || next === 'FAILED')
                setFilters((previous) => ({
                  ...previous,
                  status: next,
                  page: 1,
                }));
            }}
          />
        </div>
      </Section>

      <Section title="Completed deliveries">
        <p
          role="status"
          aria-atomic="true"
          className="text-body text-text-muted"
        >
          {result
            ? `${result.pagination.totalItems} ${status.toLowerCase()} ${result.pagination.totalItems === 1 ? 'delivery' : 'deliveries'} on ${date}.`
            : ''}
        </p>

        {!date ? (
          <EmptyState title="Choose a delivery date">
            Select a date to see completed deliveries.
          </EmptyState>
        ) : error ? (
          <HistoryError error={error} remaining={remaining} />
        ) : (
          <DataTable
            caption="Completed deliveries"
            columns={columns}
            rows={result?.items ?? []}
            getRowId={(delivery) => delivery.id}
            isLoading={isLoading}
            empty={
              <EmptyState title={`No ${status.toLowerCase()} deliveries`}>
                Choose another date or status to check other deliveries.
              </EmptyState>
            }
          />
        )}

        {showPagination && (
          <Pagination
            aria-label="Delivery history pages"
            className="flex-wrap justify-between gap-4"
          >
            <p className="text-body text-text-muted">
              {isLoading ? 'Loading page' : 'Page'}{' '}
              <span className="font-mono tabular">{page}</span>{' '}
              {result && (
                <>
                  of{' '}
                  <span className="font-mono tabular">
                    {result.pagination.totalPages}
                  </span>
                </>
              )}
            </p>
            <PaginationContent className="gap-2">
              <PaginationItem>
                <PaginationPrevious
                  size="lg"
                  className="min-w-12"
                  disabled={isLoading || page <= 1}
                  onClick={() =>
                    setFilters((previous) => ({
                      ...previous,
                      page: previous.page - 1,
                    }))
                  }
                />
              </PaginationItem>
              <PaginationItem>
                <PaginationNext
                  size="lg"
                  className="min-w-12"
                  disabled={
                    isLoading || !result || page >= result.pagination.totalPages
                  }
                  onClick={() =>
                    setFilters((previous) => ({
                      ...previous,
                      page: previous.page + 1,
                    }))
                  }
                />
              </PaginationItem>
            </PaginationContent>
          </Pagination>
        )}
      </Section>
    </>
  );
}

function HistoryError({
  error,
  remaining,
}: {
  error: ApiError;
  remaining: number;
}) {
  if (error.code === 'UNAUTHENTICATED' || error.code === 'ACCOUNT_DEACTIVATED')
    return null;
  if (error.code === 'FORBIDDEN')
    return (
      <Alert tone="danger" title="You do not have access to delivery history">
        This view is limited to couriers. Contact your dispatcher if you need
        access.
      </Alert>
    );
  if (error.code === 'NOT_FOUND')
    return (
      <Alert tone="info" title="Delivery history is not available yet">
        Please try again later.
      </Alert>
    );
  if (error.code === 'RATE_LIMITED')
    return <RateLimitedAlert remaining={remaining} />;
  if (error.isServerFault)
    return (
      <ServerFaultAlert
        isNetwork={error.code === 'NETWORK'}
        requestId={error.requestId}
      />
    );
  return (
    <Alert tone="danger" title="History could not be loaded">
      Check the selected date and status, then try again.
    </Alert>
  );
}
