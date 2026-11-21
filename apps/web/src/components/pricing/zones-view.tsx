'use client';

import { useCallback, useState } from 'react';

import type { ApiError } from '../../lib/api';
import {
  listAdminZones,
  PRICING_PAGE_SIZE,
  updateZone,
  type AdminZone,
} from '../../lib/pricing-api';
import { PageHeader } from '../layout/page-header';
import { Section } from '../layout/section';
import { Button } from '../ui/button';
import {
  DataTable,
  TablePagination,
  type DataTableColumn,
} from '../ui/data-table';
import { EmptyState } from '../ui/empty-state';
import { Select } from '../ui/select';
import { StatusChip } from '../ui/status-chip';
import { useToast } from '../ui/toast';
import { DeactivateModal } from './deactivate-modal';
import { FailureAlert } from './failure-alert';
import { asApiError, usePagedList } from './use-paged-list';
import { ZoneFormModal } from './zone-form-modal';

type StatusFilter = '' | 'ACTIVE' | 'DEACTIVATED';

const statusOptions = [
  { value: '', label: 'All zones' },
  { value: 'ACTIVE', label: 'Active' },
  { value: 'DEACTIVATED', label: 'Deactivated' },
];

/** /admin/zones: every delivery zone, active or not. */
export function ZonesView() {
  const { show } = useToast();
  const [status, setStatus] = useState<StatusFilter>('');
  const [form, setForm] = useState<{
    isOpen: boolean;
    zone: AdminZone | null;
  }>({ isOpen: false, zone: null });
  const [confirming, setConfirming] = useState<AdminZone | null>(null);
  const [pendingId, setPendingId] = useState<string | null>(null);
  const [actionError, setActionError] = useState<ApiError | null>(null);

  const load = useCallback(
    (page: number, signal: AbortSignal) =>
      listAdminZones(
        {
          isActive: status === '' ? undefined : status === 'ACTIVE',
          page,
          pageSize: PRICING_PAGE_SIZE,
          sort: 'createdAt:asc',
        },
        signal,
      ),
    [status],
  );
  const list = usePagedList(load);

  function changeStatus(value: string) {
    setActionError(null);
    list.setPage(1);
    setStatus(value as StatusFilter);
  }

  async function changeActive(zone: AdminZone, isActive: boolean) {
    setPendingId(zone.id);
    setActionError(null);
    try {
      await updateZone(zone.id, { isActive });
      list.refresh();
      show({
        title: isActive
          ? `${zone.name} is active again.`
          : `${zone.name} is deactivated.`,
      });
    } catch (error) {
      setActionError(asApiError(error));
    } finally {
      setPendingId(null);
    }
  }

  const columns: DataTableColumn<AdminZone>[] = [
    {
      key: 'code',
      header: 'Code',
      isMono: true,
      cell: (zone) => zone.code,
    },
    { key: 'name', header: 'Name', cell: (zone) => zone.name },
    {
      key: 'districts',
      header: 'Districts',
      cell: (zone) => zone.districts.join(', '),
    },
    {
      key: 'status',
      header: 'Status',
      cell: (zone) => (
        <StatusChip status={zone.isActive ? 'ACTIVE' : 'DEACTIVATED'} />
      ),
    },
    {
      key: 'actions',
      header: 'Actions',
      isHeaderHidden: true,
      cell: (zone) => (
        <div className="flex flex-wrap gap-3">
          <Button
            variant="secondary"
            size="lg"
            aria-label={`Edit ${zone.name}`}
            onClick={() => setForm({ isOpen: true, zone })}
          >
            Edit
          </Button>
          <Button
            variant="secondary"
            size="lg"
            isLoading={pendingId === zone.id}
            aria-label={`${zone.isActive ? 'Deactivate' : 'Activate'} ${zone.name}`}
            onClick={() =>
              zone.isActive ? setConfirming(zone) : changeActive(zone, true)
            }
          >
            {zone.isActive ? 'Deactivate' : 'Activate'}
          </Button>
        </div>
      ),
    },
  ];

  return (
    <>
      <PageHeader
        title="Zones"
        description="The delivery zones that pricing and parcel booking use."
        actions={
          <Button
            size="lg"
            onClick={() => setForm({ isOpen: true, zone: null })}
          >
            Add zone
          </Button>
        }
      />
      <Section
        title="All zones"
        description="A deactivated zone leaves the zone lists and new estimates."
      >
        <div className="flex flex-wrap items-end gap-3">
          <Select
            id="zone-status"
            label="Status"
            controlSize="lg"
            options={statusOptions}
            value={status}
            onChange={(event) => changeStatus(event.target.value)}
          />
        </div>

        {actionError && <FailureAlert error={actionError} />}

        {list.error ? (
          <FailureAlert error={list.error} onRetry={list.refresh} />
        ) : (
          <>
            <DataTable
              caption="Zones"
              columns={columns}
              rows={list.rows ?? []}
              getRowId={(zone) => zone.id}
              isLoading={list.isLoading}
              empty={
                status ? (
                  <EmptyState
                    title="No zones match"
                    action={
                      <Button
                        variant="secondary"
                        size="lg"
                        onClick={() => changeStatus('')}
                      >
                        Clear the filter
                      </Button>
                    }
                  >
                    Try a different status.
                  </EmptyState>
                ) : (
                  <EmptyState
                    title="No zones yet"
                    action={
                      <Button
                        variant="secondary"
                        size="lg"
                        onClick={() => setForm({ isOpen: true, zone: null })}
                      >
                        Add a zone
                      </Button>
                    }
                  >
                    Add the first zone so you can set prices for it.
                  </EmptyState>
                )
              }
            />
            {list.pagination && (
              <TablePagination
                page={list.page}
                totalPages={list.pagination.totalPages}
                totalItems={list.pagination.totalItems}
                onPageChange={(next) => {
                  setActionError(null);
                  list.setPage(next);
                }}
              />
            )}
          </>
        )}
      </Section>

      <ZoneFormModal
        isOpen={form.isOpen}
        zone={form.zone}
        onClose={() => setForm((current) => ({ ...current, isOpen: false }))}
        onSaved={(saved) => {
          const isNew = form.zone === null;
          setForm((current) => ({ ...current, isOpen: false }));
          list.refresh();
          show({
            title: isNew ? `${saved.name} added.` : `${saved.name} saved.`,
          });
        }}
      />

      <DeactivateModal
        name={confirming?.name ?? null}
        description="It leaves the zone lists and new estimates until you activate it again. Parcels already stored keep their fee."
        onClose={() => setConfirming(null)}
        onConfirm={() => {
          const zone = confirming;
          setConfirming(null);
          if (zone) void changeActive(zone, false);
        }}
      />
    </>
  );
}
