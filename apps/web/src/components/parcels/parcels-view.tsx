'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import type { ApiMeta } from '../../lib/api';
import { formatDateTime, formatLkr } from '../../lib/format';
import {
  listParcels,
  type ParcelSummary,
  type ParcelQuery,
} from '../../lib/parcels-api';
import { useSession } from '../../lib/session';
import { parcelStatuses, type ParcelStatus } from '../../lib/tokens';
import { PageHeader } from '../layout/page-header';
import { RequireRole } from '../layout/require-role';
import { Section } from '../layout/section';
import { Alert } from '../ui/alert';
import { Button, ButtonLink } from '../ui/button';
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
import { SkeletonGroup, SkeletonText } from '../ui/skeleton';
import { StatusChip, statusLabel } from '../ui/status-chip';
import { ParcelProblem } from './parcel-problem';

export function ParcelsView() {
  const { session } = useSession();
  if (session.status !== 'authenticated') return null;
  // This shared route leaves the dispatcher's future interface separate from
  // the customer-owned endpoint. No dispatcher implementation exists yet.
  if (session.user.role === 'DISPATCHER' || session.user.role === 'ADMIN')
    return (
      <>
        <PageHeader
          title="All parcels"
          description="Parcel management for dispatchers."
        />
        <Section title="Dispatch workspace">
          <EmptyState
            title="Parcel management is not available yet"
            action={
              <ButtonLink href="/reports" variant="secondary">
                View reports
              </ButtonLink>
            }
          >
            The dispatcher parcel workspace is still being built.
          </EmptyState>
        </Section>
      </>
    );
  return (
    <RequireRole roles={['CUSTOMER']} audience="customers">
      <CustomerParcels />
    </RequireRole>
  );
}

type ListState =
  | { status: 'loading' }
  | { status: 'ready'; parcels: ParcelSummary[]; meta: ApiMeta }
  | { status: 'error'; error: unknown };

function CustomerParcels() {
  const [query, setQuery] = useState<ParcelQuery>({
    page: 1,
    pageSize: 20,
    status: '',
    from: '',
    to: '',
    sort: 'createdAt:desc',
  });
  const [state, setState] = useState<ListState>({ status: 'loading' });
  const [retry, setRetry] = useState(0);
  const invalidDates = Boolean(query.from && query.to && query.from > query.to);
  useEffect(() => {
    if (invalidDates) return;
    const controller = new AbortController();
    listParcels(query, controller.signal)
      .then(({ data, meta }) =>
        setState({ status: 'ready', parcels: data, meta }),
      )
      .catch((error: unknown) => {
        if (!controller.signal.aborted) setState({ status: 'error', error });
      });
    return () => controller.abort();
  }, [query, retry, invalidDates]);
  function filter(change: Partial<ParcelQuery>) {
    setState({ status: 'loading' });
    setQuery((current) => ({ ...current, ...change, page: 1 }));
  }
  const pagination =
    state.status === 'ready' ? state.meta.pagination : undefined;
  return (
    <>
      <PageHeader
        title="My parcels"
        description="Follow your deliveries from collection to arrival."
        actions={<ButtonLink href="/new">New delivery</ButtonLink>}
      />
      <Section title="Your deliveries">
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
          <Select
            id="parcels-status"
            label="Status"
            value={query.status}
            options={[
              { value: '', label: 'All statuses' },
              ...parcelStatuses.map((status) => ({
                value: status,
                label: statusLabel(status),
              })),
            ]}
            onChange={(event) =>
              filter({ status: event.target.value as ParcelStatus | '' })
            }
          />
          <TextField
            id="parcels-from"
            label="Created from"
            type="date"
            value={query.from}
            onChange={(event) => filter({ from: event.target.value })}
          />
          <TextField
            id="parcels-to"
            label="Created to"
            type="date"
            value={query.to}
            onChange={(event) => filter({ to: event.target.value })}
            error={
              invalidDates
                ? 'Choose an end date on or after the start date.'
                : undefined
            }
          />
          <Select
            id="parcels-sort"
            label="Order"
            value={query.sort}
            options={[
              { value: 'createdAt:desc', label: 'Newest first' },
              { value: 'createdAt:asc', label: 'Oldest first' },
            ]}
            onChange={(event) =>
              filter({ sort: event.target.value as ParcelQuery['sort'] })
            }
          />
        </div>
        {invalidDates ? (
          <Alert tone="warning" title="Check the date range">
            Check the date range to show your deliveries.
          </Alert>
        ) : state.status === 'loading' ? (
          <SkeletonGroup label="Loading your parcels">
            <SkeletonText lines={3} />
          </SkeletonGroup>
        ) : state.status === 'error' ? (
          <>
            <ParcelProblem error={state.error} />
            <Button
              variant="secondary"
              onClick={() => {
                setState({ status: 'loading' });
                setRetry((n) => n + 1);
              }}
            >
              Try again
            </Button>
          </>
        ) : state.parcels.length === 0 ? (
          <EmptyState
            title={
              query.status || query.from || query.to
                ? 'No matching parcels'
                : 'No parcels yet'
            }
          >
            {query.status || query.from || query.to
              ? 'Try another status or date range.'
              : 'Use New delivery to arrange your first pickup.'}
          </EmptyState>
        ) : (
          <>
            <p role="status" className="text-body-sm text-text-muted">
              {pagination?.totalItems ?? state.parcels.length} parcel
              {(pagination?.totalItems ?? state.parcels.length) === 1
                ? ''
                : 's'}
            </p>
            <ul className="grid gap-4 md:grid-cols-2">
              {state.parcels.map((parcel) => (
                <li
                  key={parcel.id}
                  className="flex min-w-0 flex-col gap-4 rounded-card border border-line bg-surface p-5"
                >
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <Link
                      href={`/parcels/${parcel.id}`}
                      className="break-all font-mono text-body-sm text-interactive underline underline-offset-4"
                      aria-label={`View parcel ${parcel.trackingNumber}`}
                    >
                      {parcel.trackingNumber}
                    </Link>
                    <StatusChip status={parcel.status} />
                  </div>
                  <div className="flex flex-col gap-1">
                    <p className="break-words">{parcel.recipientName}</p>
                    <p className="break-words text-body-sm text-text-muted">
                      {parcel.dropoffCity}
                    </p>
                  </div>
                  <div className="flex flex-wrap justify-between gap-3">
                    <time
                      dateTime={parcel.createdAt}
                      className="font-mono text-caption text-text-muted"
                    >
                      {formatDateTime(parcel.createdAt)}
                    </time>
                    <span className="font-mono text-body-sm tabular">
                      {formatLkr(parcel.total)}
                    </span>
                  </div>
                </li>
              ))}
            </ul>
            {pagination && pagination.totalPages > 1 && (
              <Pagination>
                <PaginationContent>
                  <PaginationItem>
                    <PaginationPrevious
                      disabled={pagination.page <= 1}
                      onClick={() => {
                        setState({ status: 'loading' });
                        setQuery((current) => ({
                          ...current,
                          page: pagination.page - 1,
                        }));
                      }}
                    />
                  </PaginationItem>
                  <PaginationItem>
                    <span className="px-3 font-mono text-caption">
                      {pagination.page} / {pagination.totalPages}
                    </span>
                  </PaginationItem>
                  <PaginationItem>
                    <PaginationNext
                      disabled={pagination.page >= pagination.totalPages}
                      onClick={() => {
                        setState({ status: 'loading' });
                        setQuery((current) => ({
                          ...current,
                          page: pagination.page + 1,
                        }));
                      }}
                    />
                  </PaginationItem>
                </PaginationContent>
              </Pagination>
            )}
          </>
        )}
      </Section>
    </>
  );
}
