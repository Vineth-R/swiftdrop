'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';

import type { ApiError } from '../../lib/api';
import { formatLkr } from '../../lib/format';
import {
  getZones,
  listAdminZones,
  listPricingRules,
  PRICING_MAX_PAGE_SIZE,
  PRICING_PAGE_SIZE,
  updatePricingRule,
  type AdminZone,
  type PricingRule,
  type Zone,
} from '../../lib/pricing-api';
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
import { RuleFormModal } from './rule-form-modal';
import { asApiError, usePagedList } from './use-paged-list';

type StatusFilter = '' | 'ACTIVE' | 'DEACTIVATED';

const statusOptions = [
  { value: '', label: 'All rules' },
  { value: 'ACTIVE', label: 'Active' },
  { value: 'DEACTIVATED', label: 'Deactivated' },
];

/** Base fees per pair of zones, shown by zone name and never by id. */
export function PricingRulesSection() {
  const { show } = useToast();
  // Every zone names the rows. Only active zones can be chosen for a new rule.
  // ponytail: the first 100 zones (the API's page limit); page through them if
  // there are ever more.
  const [allZones, setAllZones] = useState<AdminZone[]>([]);
  const [activeZones, setActiveZones] = useState<Zone[]>([]);
  const [zonesError, setZonesError] = useState<ApiError | null>(null);
  const [zonesReload, setZonesReload] = useState(0);
  const [origin, setOrigin] = useState('');
  const [destination, setDestination] = useState('');
  const [status, setStatus] = useState<StatusFilter>('');
  const [form, setForm] = useState<{
    isOpen: boolean;
    rule: PricingRule | null;
  }>({ isOpen: false, rule: null });
  const [confirming, setConfirming] = useState<PricingRule | null>(null);
  const [pendingId, setPendingId] = useState<string | null>(null);
  const [actionError, setActionError] = useState<ApiError | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    setZonesError(null);
    Promise.all([
      listAdminZones(
        { pageSize: PRICING_MAX_PAGE_SIZE, sort: 'createdAt:asc' },
        controller.signal,
      ),
      getZones(controller.signal),
    ])
      .then(([all, active]) => {
        setAllZones(all.data);
        setActiveZones(active);
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) setZonesError(asApiError(error));
      });
    return () => controller.abort();
  }, [zonesReload]);

  const load = useCallback(
    (page: number, signal: AbortSignal) =>
      listPricingRules(
        {
          originZoneId: origin || undefined,
          destinationZoneId: destination || undefined,
          isActive: status === '' ? undefined : status === 'ACTIVE',
          page,
          pageSize: PRICING_PAGE_SIZE,
          sort: 'createdAt:asc',
        },
        signal,
      ),
    [origin, destination, status],
  );
  const list = usePagedList(load);

  const zoneName = useMemo(() => {
    const names = new Map(allZones.map((zone) => [zone.id, zone.name]));
    return (id: string) => names.get(id) ?? 'Unknown zone';
  }, [allZones]);
  const zoneOptions = useMemo(
    () => [
      { value: '', label: 'All zones' },
      ...allZones.map((zone) => ({ value: zone.id, label: zone.name })),
    ],
    [allZones],
  );

  const hasFilters = origin !== '' || destination !== '' || status !== '';

  function routeOf(rule: PricingRule): string {
    return `${zoneName(rule.originZoneId)} to ${zoneName(rule.destinationZoneId)}`;
  }

  function filterBy(change: () => void) {
    setActionError(null);
    list.setPage(1);
    change();
  }

  async function changeActive(rule: PricingRule, isActive: boolean) {
    setPendingId(rule.id);
    setActionError(null);
    try {
      await updatePricingRule(rule.id, { isActive });
      list.refresh();
      show({
        title: isActive
          ? `${routeOf(rule)} is active again.`
          : `${routeOf(rule)} is deactivated.`,
      });
    } catch (error) {
      setActionError(asApiError(error));
    } finally {
      setPendingId(null);
    }
  }

  const columns: DataTableColumn<PricingRule>[] = [
    {
      key: 'origin',
      header: 'From',
      cell: (rule) => zoneName(rule.originZoneId),
    },
    {
      key: 'destination',
      header: 'To',
      cell: (rule) => zoneName(rule.destinationZoneId),
    },
    {
      key: 'baseFee',
      header: 'Base fee',
      isNumeric: true,
      isMono: true,
      cell: (rule) => formatLkr(rule.baseFee),
    },
    {
      key: 'status',
      header: 'Status',
      cell: (rule) => (
        <StatusChip status={rule.isActive ? 'ACTIVE' : 'DEACTIVATED'} />
      ),
    },
    {
      key: 'actions',
      header: 'Actions',
      isHeaderHidden: true,
      cell: (rule) => (
        <div className="flex flex-wrap gap-3">
          <Button
            variant="secondary"
            size="lg"
            aria-label={`Edit ${routeOf(rule)}`}
            onClick={() => setForm({ isOpen: true, rule })}
          >
            Edit
          </Button>
          <Button
            variant="secondary"
            size="lg"
            isLoading={pendingId === rule.id}
            aria-label={`${rule.isActive ? 'Deactivate' : 'Activate'} ${routeOf(rule)}`}
            onClick={() =>
              rule.isActive ? setConfirming(rule) : changeActive(rule, true)
            }
          >
            {rule.isActive ? 'Deactivate' : 'Activate'}
          </Button>
        </div>
      ),
    },
  ];

  return (
    <Section
      title="Pricing rules"
      description="The base fee for each pair of zones."
      actions={
        <Button size="lg" onClick={() => setForm({ isOpen: true, rule: null })}>
          Add rule
        </Button>
      }
    >
      <div className="flex flex-wrap items-end gap-3">
        <Select
          id="rule-origin-filter"
          label="From"
          controlSize="lg"
          options={zoneOptions}
          value={origin}
          onChange={(event) => filterBy(() => setOrigin(event.target.value))}
        />
        <Select
          id="rule-destination-filter"
          label="To"
          controlSize="lg"
          options={zoneOptions}
          value={destination}
          onChange={(event) =>
            filterBy(() => setDestination(event.target.value))
          }
        />
        <Select
          id="rule-status-filter"
          label="Status"
          controlSize="lg"
          options={statusOptions}
          value={status}
          onChange={(event) =>
            filterBy(() => setStatus(event.target.value as StatusFilter))
          }
        />
      </div>

      {actionError && <FailureAlert error={actionError} />}
      {zonesError && (
        <FailureAlert
          error={zonesError}
          onRetry={() => setZonesReload((value) => value + 1)}
        />
      )}

      {list.error ? (
        <FailureAlert error={list.error} onRetry={list.refresh} />
      ) : (
        <>
          <DataTable
            caption="Pricing rules"
            columns={columns}
            rows={list.rows ?? []}
            getRowId={(rule) => rule.id}
            isLoading={list.isLoading}
            empty={
              hasFilters ? (
                <EmptyState
                  title="No rules match"
                  action={
                    <Button
                      variant="secondary"
                      size="lg"
                      onClick={() =>
                        filterBy(() => {
                          setOrigin('');
                          setDestination('');
                          setStatus('');
                        })
                      }
                    >
                      Clear the filters
                    </Button>
                  }
                >
                  Try a different pair of zones, or remove a filter.
                </EmptyState>
              ) : (
                <EmptyState
                  title="No pricing rules yet"
                  action={
                    <Button
                      variant="secondary"
                      size="lg"
                      onClick={() => setForm({ isOpen: true, rule: null })}
                    >
                      Add a rule
                    </Button>
                  }
                >
                  Add a rule so a route can be priced.
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

      <RuleFormModal
        isOpen={form.isOpen}
        rule={form.rule}
        route={form.rule ? routeOf(form.rule) : ''}
        zones={activeZones}
        onClose={() => setForm((current) => ({ ...current, isOpen: false }))}
        onSaved={(saved) => {
          const isNew = form.rule === null;
          setForm((current) => ({ ...current, isOpen: false }));
          list.refresh();
          show({
            title: isNew
              ? `Rule added for ${routeOf(saved)}.`
              : `${routeOf(saved)} saved.`,
          });
        }}
      />

      <DeactivateModal
        name={confirming ? routeOf(confirming) : null}
        description="This route can no longer be priced until you activate the rule again. Parcels already stored keep their fee."
        onClose={() => setConfirming(null)}
        onConfirm={() => {
          const rule = confirming;
          setConfirming(null);
          if (rule) void changeActive(rule, false);
        }}
      />
    </Section>
  );
}
