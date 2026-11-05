'use client';

import { useCallback, useState } from 'react';

import type { ApiError } from '../../lib/api';
import { formatKg, formatLkr } from '../../lib/format';
import {
  listWeightBands,
  PRICING_PAGE_SIZE,
  updateWeightBand,
  type WeightBand,
} from '../../lib/pricing-api';
import { Section } from '../layout/section';
import { Button } from '../ui/button';
import {
  DataTable,
  TablePagination,
  type DataTableColumn,
} from '../ui/data-table';
import { EmptyState } from '../ui/empty-state';
import { StatusChip } from '../ui/status-chip';
import { useToast } from '../ui/toast';
import { BandFormModal } from './band-form-modal';
import { DeactivateModal } from './deactivate-modal';
import { FailureAlert } from './failure-alert';
import { asApiError, usePagedList } from './use-paged-list';

function rangeOf(band: WeightBand): string {
  return `${formatKg(band.minWeightKg)} to ${formatKg(band.maxWeightKg)}`;
}

/** The weight charge: a band covers over its "from" weight, up to its "to" weight. */
export function WeightBandsSection() {
  const { show } = useToast();
  const [form, setForm] = useState<{
    isOpen: boolean;
    band: WeightBand | null;
  }>({ isOpen: false, band: null });
  const [confirming, setConfirming] = useState<WeightBand | null>(null);
  const [pendingId, setPendingId] = useState<string | null>(null);
  const [actionError, setActionError] = useState<ApiError | null>(null);

  const load = useCallback(
    (page: number, signal: AbortSignal) =>
      listWeightBands(
        { page, pageSize: PRICING_PAGE_SIZE, sort: 'createdAt:asc' },
        signal,
      ),
    [],
  );
  const list = usePagedList(load);

  async function changeActive(band: WeightBand, isActive: boolean) {
    setPendingId(band.id);
    setActionError(null);
    try {
      await updateWeightBand(band.id, { isActive });
      list.refresh();
      show({
        title: isActive
          ? `The ${rangeOf(band)} band is active again.`
          : `The ${rangeOf(band)} band is deactivated.`,
      });
    } catch (error) {
      setActionError(asApiError(error));
    } finally {
      setPendingId(null);
    }
  }

  const columns: DataTableColumn<WeightBand>[] = [
    {
      key: 'minWeightKg',
      header: 'Over',
      isNumeric: true,
      isMono: true,
      // On a phone this cell is the card's title and its header is not shown.
      cell: (band) => `Over ${formatKg(band.minWeightKg)}`,
    },
    {
      key: 'maxWeightKg',
      header: 'Up to',
      isNumeric: true,
      isMono: true,
      cell: (band) => formatKg(band.maxWeightKg),
    },
    {
      key: 'charge',
      header: 'Charge',
      isNumeric: true,
      isMono: true,
      cell: (band) => formatLkr(band.charge),
    },
    {
      key: 'status',
      header: 'Status',
      cell: (band) => (
        <StatusChip status={band.isActive ? 'ACTIVE' : 'DEACTIVATED'} />
      ),
    },
    {
      key: 'actions',
      header: 'Actions',
      isHeaderHidden: true,
      cell: (band) => (
        <div className="flex flex-wrap gap-3">
          <Button
            variant="secondary"
            size="lg"
            aria-label={`Edit the ${rangeOf(band)} band`}
            onClick={() => setForm({ isOpen: true, band })}
          >
            Edit
          </Button>
          <Button
            variant="secondary"
            size="lg"
            isLoading={pendingId === band.id}
            aria-label={`${band.isActive ? 'Deactivate' : 'Activate'} the ${rangeOf(band)} band`}
            onClick={() =>
              band.isActive ? setConfirming(band) : changeActive(band, true)
            }
          >
            {band.isActive ? 'Deactivate' : 'Activate'}
          </Button>
        </div>
      ),
    },
  ];

  return (
    <Section
      title="Weight bands"
      description="The charge added for a parcel's weight."
      actions={
        <Button
          variant="secondary"
          size="lg"
          onClick={() => setForm({ isOpen: true, band: null })}
        >
          Add band
        </Button>
      }
    >
      {actionError && <FailureAlert error={actionError} />}

      {list.error ? (
        <FailureAlert error={list.error} onRetry={list.refresh} />
      ) : (
        <>
          <DataTable
            caption="Weight bands"
            columns={columns}
            rows={list.rows ?? []}
            getRowId={(band) => band.id}
            isLoading={list.isLoading}
            empty={
              <EmptyState
                title="No weight bands yet"
                action={
                  <Button
                    variant="secondary"
                    size="lg"
                    onClick={() => setForm({ isOpen: true, band: null })}
                  >
                    Add a band
                  </Button>
                }
              >
                Add a band so a parcel's weight can be priced.
              </EmptyState>
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

      <BandFormModal
        isOpen={form.isOpen}
        band={form.band}
        loadedBands={list.rows ?? []}
        onClose={() => setForm((current) => ({ ...current, isOpen: false }))}
        onSaved={(saved) => {
          const isNew = form.band === null;
          setForm((current) => ({ ...current, isOpen: false }));
          list.refresh();
          show({
            title: isNew
              ? `The ${rangeOf(saved)} band is added.`
              : `The ${rangeOf(saved)} band is saved.`,
          });
        }}
      />

      <DeactivateModal
        name={confirming ? `the ${rangeOf(confirming)} band` : null}
        description="Parcels in this weight range can no longer be priced until you activate it again or another band covers them. Parcels already stored keep their fee."
        onClose={() => setConfirming(null)}
        onConfirm={() => {
          const band = confirming;
          setConfirming(null);
          if (band) void changeActive(band, false);
        }}
      />
    </Section>
  );
}
