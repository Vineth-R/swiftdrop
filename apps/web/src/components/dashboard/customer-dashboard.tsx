'use client';

import Link from 'next/link';

import { formatDateTime } from '@/lib/format';
import { listParcels, type ParcelSummary } from '@/lib/parcels-api';
import { useLoad } from '@/lib/use-load';
import { PageHeader } from '../layout/page-header';
import { Section } from '../layout/section';
import { ButtonLink } from '../ui/button';
import { DataTable, type DataTableColumn } from '../ui/data-table';
import { EmptyState } from '../ui/empty-state';
import { StatusChip } from '../ui/status-chip';
import { LoadError } from './load-error';

const RECENT = 5;

const columns: DataTableColumn<ParcelSummary>[] = [
  {
    key: 'trackingNumber',
    header: 'Tracking number',
    isMono: true,
    cell: (parcel) => (
      <Link
        href={`/parcels/${parcel.id}`}
        className="font-semibold text-interactive underline underline-offset-4"
      >
        {parcel.trackingNumber}
      </Link>
    ),
  },
  {
    key: 'status',
    header: 'Status',
    cell: (parcel) => <StatusChip status={parcel.status} />,
  },
  {
    key: 'recipient',
    header: 'To',
    cell: (parcel) => parcel.dropoffCity,
  },
  {
    key: 'createdAt',
    header: 'Created',
    cell: (parcel) => formatDateTime(parcel.createdAt),
  },
];

/** A customer's home: what to do next, and where their latest parcels stand. */
export function CustomerDashboard({ name }: { name: string }) {
  const { state, retry } = useLoad((signal) =>
    listParcels({ page: 1, pageSize: RECENT, sort: 'createdAt:desc' }, signal),
  );
  const total =
    state.status === 'ready'
      ? (state.data.meta.pagination?.totalItems ?? state.data.data.length)
      : 0;

  return (
    <>
      <PageHeader
        title="Dashboard"
        description={`Signed in as ${name}.`}
        actions={
          <>
            <ButtonLink href="/new">New delivery</ButtonLink>
            <ButtonLink href="/track" variant="secondary">
              Track a parcel
            </ButtonLink>
          </>
        }
      />
      <Section
        title="Recent parcels"
        description={
          total > RECENT
            ? `Your latest ${RECENT} of ${total} parcels.`
            : undefined
        }
        actions={
          <ButtonLink href="/parcels" variant="ghost" size="sm">
            All parcels
          </ButtonLink>
        }
      >
        {state.status === 'error' ? (
          <LoadError
            title="Could not load your parcels"
            error={state.error}
            onRetry={retry}
          />
        ) : (
          <DataTable
            caption="Your recent parcels"
            columns={columns}
            rows={state.status === 'ready' ? state.data.data : []}
            getRowId={(parcel) => parcel.id}
            isLoading={state.status === 'loading'}
            empty={
              <EmptyState
                title="No parcels yet"
                action={<ButtonLink href="/new">Create a delivery</ButtonLink>}
              >
                Create your first delivery and it will show here.
              </EmptyState>
            }
          />
        )}
      </Section>
    </>
  );
}
