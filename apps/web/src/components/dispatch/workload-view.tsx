'use client';

import { RotateCcw, Truck } from 'lucide-react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import type { ApiError } from '../../lib/api';
import {
  getCourierWorkload,
  type CourierWorkload,
} from '../../lib/dispatch-api';
import { useLiveUpdates, type LiveState } from '../../lib/use-live-updates';
import { PageHeader } from '../layout/page-header';
import { RequireRole } from '../layout/require-role';
import { Section } from '../layout/section';
import { Button } from '../ui/button';
import { DataTable, type DataTableColumn } from '../ui/data-table';
import { EmptyState } from '../ui/empty-state';
import { TextField } from '../ui/field';
import { Select } from '../ui/select';
import { StatusChip } from '../ui/status-chip';
import { DispatchAlert } from './dispatch-alerts';
import { asApiError } from './use-paged-list';

export function WorkloadView() {
  return (
    <RequireRole
      roles={['DISPATCHER', 'ADMIN']}
      audience="dispatchers and administrators"
    >
      <WorkloadContent />
    </RequireRole>
  );
}

const availabilityOptions = [
  { value: '', label: 'All couriers' },
  { value: 'available', label: 'Available only' },
  { value: 'unavailable', label: 'Unavailable only' },
];

/** The live connection in words, as M10's hook asks. */
const liveCaptions: Record<LiveState, string> = {
  live: 'Live. Parcel statuses change here as they happen.',
  connecting: 'Connecting to live updates. Reload the page to check now.',
  reconnecting: 'Connecting to live updates. Reload the page to check now.',
  denied: 'Live updates are off. Reload the page to check for changes.',
  limited: 'Live updates are off. Reload the page to check for changes.',
};

function WorkloadContent() {
  const [data, setData] = useState<CourierWorkload[] | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<ApiError | null>(null);

  const [search, setSearch] = useState('');
  const [availability, setAvailability] = useState('');

  const dataRef = useRef(data);
  useEffect(() => {
    dataRef.current = data;
  });

  const nextSyncId = useRef(0);
  const lastAppliedSync = useRef(0);
  const syncAbort = useRef<AbortController | null>(null);

  useEffect(() => () => syncAbort.current?.abort(), []);

  const loadData = useCallback(() => {
    const controller = new AbortController();
    setIsLoading(true);
    setError(null);

    getCourierWorkload(controller.signal)
      .then((items) => {
        setData(items);
      })
      .catch((err: unknown) => {
        if (!controller.signal.aborted) {
          setError(asApiError(err));
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setIsLoading(false);
        }
      });

    return () => controller.abort();
  }, []);

  const quietRefetch = useCallback(() => {
    if (dataRef.current === null) return undefined;
    const syncId = (nextSyncId.current += 1);
    const signal =
      syncAbort.current?.signal ??
      (syncAbort.current = new AbortController()).signal;
    return getCourierWorkload(signal)
      .then((items) => {
        if (syncId <= lastAppliedSync.current) return;
        lastAppliedSync.current = syncId;
        setData(items);
      })
      .catch(() => undefined);
  }, []);

  const { state: liveState } = useLiveUpdates({
    scope: 'all',
    onUpdate: () => {
      quietRefetch();
    },
    onSync: () => {
      quietRefetch();
    },
  });

  useEffect(() => {
    const abort = loadData();
    return abort;
  }, [loadData]);

  const filtered = useMemo(() => {
    if (!data) return [];
    return data.filter((item) => {
      if (search) {
        const query = search.toLowerCase();
        if (!item.fullName.toLowerCase().includes(query)) return false;
      }
      if (availability === 'available' && !item.isAvailable) return false;
      if (availability === 'unavailable' && item.isAvailable) return false;
      return true;
    });
  }, [data, search, availability]);

  const hasFilters = Boolean(search || availability);

  function resetFilters() {
    setSearch('');
    setAvailability('');
  }

  const columns: DataTableColumn<CourierWorkload>[] = [
    {
      key: 'fullName',
      header: 'Courier',
      cell: (item) => (
        <span className="font-semibold text-text">{item.fullName}</span>
      ),
    },
    {
      key: 'isAvailable',
      header: 'Availability',
      cell: (item) => (
        <StatusChip status={item.isAvailable ? 'ACTIVE' : 'DEACTIVATED'} />
      ),
    },
    {
      key: 'activeDeliveries',
      header: 'Active deliveries',
      isNumeric: true,
      cell: (item) => (
        <span className="font-mono text-body font-semibold text-text">
          {item.activeDeliveries}
        </span>
      ),
    },
    {
      key: 'CREATED',
      header: 'Created',
      isNumeric: true,
      cell: (item) => (
        <span className="font-mono text-body-sm text-text-muted">
          {item.byStatus?.['CREATED'] ?? 0}
        </span>
      ),
    },
    {
      key: 'PICKED_UP',
      header: 'Picked up',
      isNumeric: true,
      cell: (item) => (
        <span className="font-mono text-body-sm text-text-muted">
          {item.byStatus?.['PICKED_UP'] ?? 0}
        </span>
      ),
    },
    {
      key: 'IN_TRANSIT',
      header: 'In transit',
      isNumeric: true,
      cell: (item) => (
        <span className="font-mono text-body-sm text-text-muted">
          {item.byStatus?.['IN_TRANSIT'] ?? 0}
        </span>
      ),
    },
    {
      key: 'OUT_FOR_DELIVERY',
      header: 'Out for delivery',
      isNumeric: true,
      cell: (item) => (
        <span className="font-mono text-body-sm text-text-muted">
          {item.byStatus?.['OUT_FOR_DELIVERY'] ?? 0}
        </span>
      ),
    },
  ];

  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title="Courier workload"
        description="Active deliveries and courier capacity across the fleet."
        actions={
          <Button
            variant="secondary"
            size="md"
            onClick={loadData}
            isLoading={isLoading}
          >
            Refresh
          </Button>
        }
      />

      {liveState !== 'idle' && (
        <p role="status" className="text-body-sm text-text-muted">
          {liveCaptions[liveState]}
        </p>
      )}

      <Section
        title="Fleet workload"
        description="Active delivery counts across each parcel lifecycle stage."
      >
        <div className="flex flex-wrap items-end gap-4 rounded-card border border-line bg-surface p-4">
          <div className="w-full sm:w-64">
            <TextField
              id="workload-search"
              label="Search courier"
              placeholder="Filter by name..."
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
          </div>

          <div className="w-full sm:w-48">
            <Select
              id="workload-availability"
              label="Availability"
              options={availabilityOptions}
              value={availability}
              onChange={(event) => setAvailability(event.target.value)}
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

        {error && <DispatchAlert error={error} onRetry={loadData} />}

        {!error && (
          <DataTable
            caption="Courier workload"
            columns={columns}
            rows={filtered}
            getRowId={(item) => item.courierId}
            isLoading={isLoading}
            empty={
              <EmptyState
                icon={Truck}
                title={
                  hasFilters
                    ? 'No couriers match the filters'
                    : 'No courier workload found'
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
                  ? 'Try searching with a different name or changing the availability filter.'
                  : 'There are currently no active couriers or recorded assignments.'}
              </EmptyState>
            }
          />
        )}
      </Section>
    </div>
  );
}
