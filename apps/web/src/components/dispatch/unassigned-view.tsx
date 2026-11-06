'use client';

import { Package, RotateCcw } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';

import {
  DISPATCH_PAGE_SIZE,
  listUnassignedParcels,
  listZones,
  type UnassignedParcel,
  type Zone,
} from '../../lib/dispatch-api';
import { formatDate, formatKg } from '../../lib/format';
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
import { AssignCourierModal } from './assign-courier-modal';
import { DispatchAlert } from './dispatch-alerts';
import { usePagedList } from './use-paged-list';

export function UnassignedView() {
  return (
    <RequireRole
      roles={['DISPATCHER', 'ADMIN']}
      audience="dispatchers and administrators"
    >
      <UnassignedContent />
    </RequireRole>
  );
}

function UnassignedContent() {
  const [zones, setZones] = useState<Zone[]>([]);
  const [selectedZone, setSelectedZone] = useState('');
  const [fromDate, setFromDate] = useState('');
  const [toDate, setToDate] = useState('');

  const [assigningParcel, setAssigningParcel] =
    useState<UnassignedParcel | null>(null);

  // Load zones for the filter dropdown
  useEffect(() => {
    const controller = new AbortController();
    listZones(controller.signal)
      .then((data) => setZones(data))
      .catch(() => {
        // Fall back gracefully; filtering by zone simply remains unavailable
      });
    return () => controller.abort();
  }, []);

  const load = useCallback(
    (page: number, signal: AbortSignal) =>
      listUnassignedParcels(
        {
          zoneId: selectedZone || undefined,
          from: fromDate ? `${fromDate}T00:00:00Z` : undefined,
          to: toDate ? `${toDate}T23:59:59Z` : undefined,
          page,
          pageSize: DISPATCH_PAGE_SIZE,
          sort: 'createdAt:asc',
        },
        signal,
      ),
    [selectedZone, fromDate, toDate],
  );

  const list = usePagedList(load);

  const hasFilters = Boolean(selectedZone || fromDate || toDate);

  function resetFilters() {
    setSelectedZone('');
    setFromDate('');
    setToDate('');
    list.setPage(1);
  }

  const zoneOptions = [
    { value: '', label: 'All pickup zones' },
    ...zones.map((zone) => ({
      value: zone.id,
      label: `${zone.name} (${zone.code})`,
    })),
  ];

  const columns: DataTableColumn<UnassignedParcel>[] = [
    {
      key: 'trackingNumber',
      header: 'Tracking number',
      isMono: true,
      cell: (parcel) => (
        <span className="font-semibold text-text">{parcel.trackingNumber}</span>
      ),
    },
    {
      key: 'pickup',
      header: 'Pickup location',
      cell: (parcel) => (
        <div className="flex flex-col text-body-sm">
          <span className="text-text">{parcel.pickupAddress}</span>
          <span className="text-text-muted">{parcel.pickupCity}</span>
        </div>
      ),
    },
    {
      key: 'dropoff',
      header: 'Dropoff location',
      cell: (parcel) => (
        <div className="flex flex-col text-body-sm">
          <span className="text-text">{parcel.dropoffAddress}</span>
          <span className="text-text-muted">{parcel.dropoffCity}</span>
        </div>
      ),
    },
    {
      key: 'weightAndSize',
      header: 'Weight & Size',
      cell: (parcel) => (
        <span className="text-body-sm text-text">
          {formatKg(parcel.weightKg)} · {parcel.sizeCategory}
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
      cell: (parcel) => (
        <Button
          variant="secondary"
          size="md"
          onClick={() => setAssigningParcel(parcel)}
          aria-label={`Assign courier to parcel ${parcel.trackingNumber}`}
        >
          Assign courier
        </Button>
      ),
    },
  ];

  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title="Unassigned parcels"
        description="Parcels waiting for a courier assignment."
      />

      <Section
        title="Unassigned queue"
        description="Review waiting parcels and assign them to available couriers."
      >
        <div className="flex flex-wrap items-end gap-4 rounded-card border border-line bg-surface p-4">
          <div className="w-full sm:w-64">
            <Select
              id="filter-pickup-zone"
              label="Pickup zone"
              options={zoneOptions}
              value={selectedZone}
              onChange={(event) => {
                setSelectedZone(event.target.value);
                list.setPage(1);
              }}
            />
          </div>

          <div className="w-full sm:w-44">
            <TextField
              id="filter-from-date"
              label="From"
              type="date"
              value={fromDate}
              onChange={(event) => {
                setFromDate(event.target.value);
                list.setPage(1);
              }}
            />
          </div>

          <div className="w-full sm:w-44">
            <TextField
              id="filter-to-date"
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
              caption="Unassigned parcels"
              columns={columns}
              rows={list.rows ?? []}
              getRowId={(parcel) => parcel.id}
              isLoading={list.isLoading}
              empty={
                <EmptyState
                  icon={Package}
                  title={
                    hasFilters
                      ? 'No parcels match the filters'
                      : 'No unassigned parcels'
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
                    ? 'Try broadening your date range or selecting another zone.'
                    : 'All parcels in this queue have been assigned to couriers.'}
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
        isOpen={Boolean(assigningParcel)}
        onClose={() => setAssigningParcel(null)}
        parcel={assigningParcel}
        onAssigned={list.refresh}
      />
    </div>
  );
}
