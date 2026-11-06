'use client';

import { List, RotateCcw } from 'lucide-react';
import { useCallback, useState } from 'react';

import {
  DISPATCH_PAGE_SIZE,
  searchAllParcels,
  type AdminParcelItem,
} from '../../lib/dispatch-api';
import { formatDate, formatLkr } from '../../lib/format';
import type { ParcelStatus } from '../../lib/tokens';
import { PageHeader } from '../layout/page-header';
import { RequireRole } from '../layout/require-role';
import { Section } from '../layout/section';
import { Button } from '../ui/button';
import {
  DataTable,
  TablePagination,
  type DataTableColumn,
} from '../ui/data-table';
import { EmptyState } from '../ui/empty-state';
import { TextField } from '../ui/field';
import { Select } from '../ui/select';
import { StatusChip } from '../ui/status-chip';
import { AssignCourierModal } from './assign-courier-modal';
import { DispatchAlert } from './dispatch-alerts';
import { usePagedList } from './use-paged-list';

type ReassignTarget = {
  id: string;
  trackingNumber: string;
  currentCourierId?: string | null;
};

const statusOptions = [
  { value: '', label: 'All statuses' },
  { value: 'CREATED', label: 'Created' },
  { value: 'PICKED_UP', label: 'Picked up' },
  { value: 'IN_TRANSIT', label: 'In transit' },
  { value: 'OUT_FOR_DELIVERY', label: 'Out for delivery' },
  { value: 'DELIVERED', label: 'Delivered' },
  { value: 'FAILED', label: 'Failed' },
];

export function DispatcherParcelsView() {
  return (
    <RequireRole
      roles={['DISPATCHER', 'ADMIN']}
      audience="dispatchers and administrators"
    >
      <DispatcherParcelsContent />
    </RequireRole>
  );
}

function DispatcherParcelsContent() {
  const [trackingNumber, setTrackingNumber] = useState('');
  const [status, setStatus] = useState('');
  const [courierId, setCourierId] = useState('');
  const [merchantId, setMerchantId] = useState('');
  const [fromDate, setFromDate] = useState('');
  const [toDate, setToDate] = useState('');

  const [reassignTarget, setReassignTarget] = useState<ReassignTarget | null>(
    null,
  );

  // TODO(realtime): subscribe when M10's hook lands

  const load = useCallback(
    (page: number, signal: AbortSignal) =>
      searchAllParcels(
        {
          trackingNumber: trackingNumber.trim() || undefined,
          status: status || undefined,
          courierId: courierId.trim() || undefined,
          merchantId: merchantId.trim() || undefined,
          from: fromDate ? `${fromDate}T00:00:00Z` : undefined,
          to: toDate ? `${toDate}T23:59:59Z` : undefined,
          page,
          pageSize: DISPATCH_PAGE_SIZE,
          sort: 'createdAt:desc',
        },
        signal,
      ),
    [trackingNumber, status, courierId, merchantId, fromDate, toDate],
  );

  const list = usePagedList(load);

  const hasFilters = Boolean(
    trackingNumber || status || courierId || merchantId || fromDate || toDate,
  );

  function resetFilters() {
    setTrackingNumber('');
    setStatus('');
    setCourierId('');
    setMerchantId('');
    setFromDate('');
    setToDate('');
    list.setPage(1);
  }

  const columns: DataTableColumn<AdminParcelItem>[] = [
    {
      key: 'trackingNumber',
      header: 'Tracking number',
      isMono: true,
      cell: (parcel) => (
        <span className="font-semibold text-text">{parcel.trackingNumber}</span>
      ),
    },
    {
      key: 'status',
      header: 'Status',
      cell: (parcel) => (
        <StatusChip status={parcel.status as ParcelStatus} />
      ),
    },
    {
      key: 'pickupCity',
      header: 'Pickup city',
      cell: (parcel) => (
        <span className="text-body-sm text-text">{parcel.pickupCity}</span>
      ),
    },
    {
      key: 'dropoffCity',
      header: 'Dropoff city',
      cell: (parcel) => (
        <span className="text-body-sm text-text">{parcel.dropoffCity}</span>
      ),
    },
    {
      key: 'total',
      header: 'Fee',
      isNumeric: true,
      cell: (parcel) => (
        <span className="font-mono text-body-sm text-text">
          {formatLkr(parcel.total)}
        </span>
      ),
    },
    {
      key: 'createdAt',
      header: 'Created',
      cell: (parcel) => (
        <span className="text-body-sm text-text-muted">
          {formatDate(parcel.createdAt)}
        </span>
      ),
    },
    {
      key: 'actions',
      header: 'Actions',
      isHeaderHidden: true,
      cell: (parcel) => {
        const isClosed =
          parcel.status === 'DELIVERED' || parcel.status === 'FAILED';
        if (isClosed) {
          return (
            <span className="text-caption text-text-muted">Closed</span>
          );
        }
        return (
          <Button
            variant="secondary"
            size="md"
            onClick={() =>
              setReassignTarget({
                id: parcel.id,
                trackingNumber: parcel.trackingNumber,
                currentCourierId: parcel.courierId,
              })
            }
            aria-label={
              parcel.courierId
                ? `Reassign parcel ${parcel.trackingNumber}`
                : `Assign courier to parcel ${parcel.trackingNumber}`
            }
          >
            {parcel.courierId ? 'Reassign' : 'Assign'}
          </Button>
        );
      },
    },
  ];

  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title="All parcels"
        description="Search, filter, and manage all parcels across the network."
      />

      <Section
        title="Parcel directory"
        description="Filter by tracking number, status, courier, merchant, or date range."
      >
        <div className="flex flex-wrap items-end gap-4 rounded-card border border-line bg-surface p-4">
          <div className="w-full sm:w-56">
            <TextField
              id="search-tracking-number"
              label="Tracking number"
              placeholder="e.g. SWD-12345678"
              value={trackingNumber}
              onChange={(event) => {
                setTrackingNumber(event.target.value);
                list.setPage(1);
              }}
            />
          </div>

          <div className="w-full sm:w-44">
            <Select
              id="filter-parcel-status"
              label="Status"
              options={statusOptions}
              value={status}
              onChange={(event) => {
                setStatus(event.target.value);
                list.setPage(1);
              }}
            />
          </div>

          <div className="w-full sm:w-52">
            <TextField
              id="filter-courier-id"
              label="Courier ID"
              placeholder="Filter by courier..."
              value={courierId}
              onChange={(event) => {
                setCourierId(event.target.value);
                list.setPage(1);
              }}
            />
          </div>

          <div className="w-full sm:w-52">
            <TextField
              id="filter-merchant-id"
              label="Merchant ID"
              placeholder="Filter by merchant..."
              value={merchantId}
              onChange={(event) => {
                setMerchantId(event.target.value);
                list.setPage(1);
              }}
            />
          </div>

          <div className="w-full sm:w-40">
            <TextField
              id="filter-all-from"
              label="From"
              type="date"
              value={fromDate}
              onChange={(event) => {
                setFromDate(event.target.value);
                list.setPage(1);
              }}
            />
          </div>

          <div className="w-full sm:w-40">
            <TextField
              id="filter-all-to"
              label="To"
              type="date"
              value={toDate}
              onChange={(event) => {
                setToDate(event.target.value);
                list.setPage(1);
              }}
            />
          </div>

          {hasFilters && (
            <div className="flex items-center">
              <Button
                variant="ghost"
                size="md"
                onClick={resetFilters}
                className="flex items-center gap-2"
              >
                <RotateCcw size={16} aria-hidden="true" />
                Reset filters
              </Button>
            </div>
          )}
        </div>

        {list.error && (
          <DispatchAlert error={list.error} onRetry={list.refresh} />
        )}

        {!list.error && (
          <>
            <DataTable
              caption="All parcels"
              columns={columns}
              rows={list.rows ?? []}
              getRowId={(parcel) => parcel.id}
              isLoading={list.isLoading}
              empty={
                <EmptyState
                  icon={List}
                  title={
                    hasFilters
                      ? 'No parcels match the filters'
                      : 'No parcels found'
                  }
                  action={
                    hasFilters ? (
                      <Button variant="secondary" onClick={resetFilters}>
                        Clear filters
                      </Button>
                    ) : undefined
                  }
                >
                  {hasFilters
                    ? 'Try broadening your search or resetting some of the filters.'
                    : 'There are currently no parcels recorded in the system.'}
                </EmptyState>
              }
            />

            {list.pagination && (
              <TablePagination
                page={list.page}
                totalPages={list.pagination.totalPages}
                totalItems={list.pagination.totalItems}
                onPageChange={list.setPage}
                isLoading={list.isLoading}
              />
            )}
          </>
        )}
      </Section>

      <AssignCourierModal
        isOpen={Boolean(reassignTarget)}
        onClose={() => setReassignTarget(null)}
        parcel={reassignTarget}
        onAssigned={list.refresh}
      />
    </div>
  );
}
