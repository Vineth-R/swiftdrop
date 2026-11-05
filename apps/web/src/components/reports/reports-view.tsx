'use client';

import { useEffect, useState, type FormEvent } from 'react';

import { ApiError } from '../../lib/api';
import {
  MAX_RANGE_DAYS,
  defaultRange,
  formatReportDate,
  getDailyReport,
  getOutcomesReport,
  validateRange,
  type DailyRow,
  type DateRange,
  type OutcomesReport,
  type RangeErrors,
} from '../../lib/reports-api';
import { PageHeader } from '../layout/page-header';
import { RequireRole } from '../layout/require-role';
import { Section } from '../layout/section';
import { Alert } from '../ui/alert';
import { Button } from '../ui/button';
import { Card, CardContent } from '../ui/card';
import { DataTable, type DataTableColumn } from '../ui/data-table';
import { EmptyState } from '../ui/empty-state';
import { TextField } from '../ui/field';
import { KeyValueList } from '../ui/key-value-list';
import { Skeleton, SkeletonGroup } from '../ui/skeleton';
import { DailyChart } from './daily-chart';

type Loaded = { daily: DailyRow[]; outcomes: OutcomesReport };

type State =
  | { status: 'loading' }
  | { status: 'error'; error: ApiError }
  | ({ status: 'ready' } & Loaded);

const INTRO = 'Parcel volumes and delivery outcomes for the dates you choose.';

/** /reports: parcel volumes and delivery outcomes. Dispatchers and administrators. */
export function ReportsView() {
  return (
    <RequireRole
      roles={['DISPATCHER']}
      audience="dispatchers and administrators"
    >
      <ReportsContent />
    </RequireRole>
  );
}

function ReportsContent() {
  // Created here, below RequireRole, so "today" is read in the browser only.
  const [range, setRange] = useState<DateRange>(() => defaultRange());
  const [draft, setDraft] = useState<DateRange>(range);
  const [errors, setErrors] = useState<RangeErrors>({});
  const [attempt, setAttempt] = useState(0);
  const [state, setState] = useState<State>({ status: 'loading' });

  useEffect(() => {
    const controller = new AbortController();
    Promise.all([
      getDailyReport(range, controller.signal),
      getOutcomesReport(range, controller.signal),
    ])
      .then(([daily, outcomes]) => {
        if (!controller.signal.aborted) {
          setState({ status: 'ready', daily, outcomes });
        }
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        setState({
          status: 'error',
          error:
            error instanceof ApiError
              ? error
              : new ApiError({ code: 'INTERNAL', status: 0 }),
        });
      });
    return () => controller.abort();
  }, [range, attempt]);

  function load(next: DateRange) {
    setState({ status: 'loading' });
    // A new object every time, so applying the same dates again reloads.
    setRange({ ...next });
  }

  function apply(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const problems = validateRange(draft);
    setErrors(problems);
    if (Object.keys(problems).length > 0) return;
    load(draft);
  }

  function retry() {
    setState({ status: 'loading' });
    setAttempt((count) => count + 1);
  }

  function reset() {
    const initial = defaultRange();
    setDraft(initial);
    setErrors({});
    load(initial);
  }

  return (
    <>
      <PageHeader title="Reports" description={describe(state)} />

      <Section
        title="Date range"
        description={`Dates are UTC. A range can be up to ${MAX_RANGE_DAYS} days.`}
      >
        <form
          noValidate
          aria-label="Report date range"
          onSubmit={apply}
          className="flex flex-col gap-4 sm:flex-row sm:flex-wrap sm:items-start"
        >
          <TextField
            id="reports-from"
            label="From"
            type="date"
            value={draft.from}
            max={draft.to || undefined}
            error={errors.from}
            onChange={(event) =>
              setDraft((current) => ({ ...current, from: event.target.value }))
            }
          />
          <TextField
            id="reports-to"
            label="To"
            type="date"
            value={draft.to}
            min={draft.from || undefined}
            error={errors.to}
            onChange={(event) =>
              setDraft((current) => ({ ...current, to: event.target.value }))
            }
          />
          <Button
            type="submit"
            className="sm:mt-6"
            isLoading={state.status === 'loading'}
          >
            {state.status === 'loading' ? 'Loading' : 'Apply range'}
          </Button>
        </form>
      </Section>

      {state.status === 'loading' && <ReportsSkeleton />}
      {state.status === 'error' && (
        <ReportsError error={state.error} onRetry={retry} />
      )}
      {state.status === 'ready' && isEmpty(state) && (
        <EmptyState
          title="No parcels in this range"
          action={
            <Button variant="secondary" onClick={reset}>
              Reset range
            </Button>
          }
        >
          Nothing was created, delivered or failed between{' '}
          {formatReportDate(range.from)} and {formatReportDate(range.to)}. Try
          a wider range.
        </EmptyState>
      )}
      {state.status === 'ready' && !isEmpty(state) && (
        <ReportsReady daily={state.daily} outcomes={state.outcomes} />
      )}
    </>
  );
}

/** The one sentence under the title: what the numbers below add up to. */
function describe(state: State): string {
  if (state.status !== 'ready') return INTRO;
  const { totals, from, to } = state.outcomes;
  const period = `from ${formatReportDate(from)} to ${formatReportDate(to)}`;
  if (totals.created === 0) return `No parcels were created ${period}.`;
  return `${totals.delivered} of ${totals.created} parcels created ${period} were delivered. ${totals.failed} failed and ${totals.inProgress} are still in progress.`;
}

function isEmpty(state: Loaded) {
  return (
    state.outcomes.totals.created === 0 &&
    state.daily.every(
      (row) => row.created === 0 && row.delivered === 0 && row.failed === 0,
    )
  );
}

/** Grey blocks in the same places as the real content, so nothing jumps. */
function ReportsSkeleton() {
  return (
    <>
      <Section title="Outcomes">
        <SkeletonGroup label="Loading outcomes" className="flex flex-col gap-4">
          <Skeleton className="h-32 w-full" />
          <Skeleton className="h-24 w-full" />
        </SkeletonGroup>
      </Section>
      <Section title="Daily parcels">
        <SkeletonGroup
          label="Loading daily parcels"
          className="flex flex-col gap-4"
        >
          <Skeleton className="h-48 w-full" />
          <Skeleton className="h-32 w-full" />
        </SkeletonGroup>
      </Section>
    </>
  );
}

function ReportsError({
  error,
  onRetry,
}: {
  error: ApiError;
  onRetry: () => void;
}) {
  const message =
    error.code === 'NETWORK'
      ? 'Check your connection, then try again.'
      : error.code === 'VALIDATION_FAILED'
        ? 'The server did not accept that date range. Change the dates and apply them again.'
        : error.code === 'FORBIDDEN'
          ? 'Your account is not allowed to see reports.'
          : 'Something went wrong on our side. Try again in a moment.';

  return (
    <Alert
      tone="danger"
      title="Reports could not be loaded"
      actions={
        <Button variant="secondary" size="sm" onClick={onRetry}>
          Try again
        </Button>
      }
    >
      {message}
      {error.isServerFault && error.requestId && (
        <>
          {' '}
          Reference: <span className="font-mono">{error.requestId}</span>
        </>
      )}
    </Alert>
  );
}

const dailyColumns: DataTableColumn<DailyRow>[] = [
  {
    key: 'date',
    header: 'Date',
    cell: (row) => formatReportDate(row.date),
    isMono: true,
  },
  {
    key: 'created',
    header: 'Created',
    cell: (row) => row.created,
    isNumeric: true,
  },
  {
    key: 'delivered',
    header: 'Delivered',
    cell: (row) => row.delivered,
    isNumeric: true,
  },
  {
    key: 'failed',
    header: 'Failed',
    cell: (row) => row.failed,
    isNumeric: true,
  },
];

type Reason = OutcomesReport['topFailureReasons'][number];

const reasonColumns: DataTableColumn<Reason>[] = [
  { key: 'reason', header: 'Reason', cell: (row) => row.reason },
  {
    key: 'count',
    header: 'Parcels',
    cell: (row) => row.count,
    isNumeric: true,
  },
];

function percent(value: number) {
  return `${value.toFixed(2)}%`;
}

function ReportsReady({ daily, outcomes }: Loaded) {
  const { totals } = outcomes;
  return (
    <>
      <Section
        title="Outcomes"
        description="Parcels created in the range, by their status now. Rates count finished parcels only: delivered plus failed."
      >
        <Card>
          <CardContent className="grid gap-6 sm:grid-cols-2">
            <KeyValueList
              layout="stacked"
              items={[
                { label: 'Created', value: totals.created, isMono: true },
                { label: 'Delivered', value: totals.delivered, isMono: true },
                { label: 'Failed', value: totals.failed, isMono: true },
                {
                  label: 'In progress',
                  value: totals.inProgress,
                  isMono: true,
                },
              ]}
            />
            <KeyValueList
              layout="stacked"
              items={[
                {
                  label: 'Success rate',
                  value: percent(outcomes.successRate),
                  isMono: true,
                },
                {
                  label: 'Failure rate',
                  value: percent(outcomes.failureRate),
                  isMono: true,
                },
              ]}
            />
          </CardContent>
        </Card>
        <h3 className="text-h4">Top failure reasons</h3>
        <DataTable
          caption="Top failure reasons"
          columns={reasonColumns}
          rows={outcomes.topFailureReasons}
          getRowId={(row) => row.reason}
          empty={
            <EmptyState title="No failed parcels">
              {totals.delivered > 0
                ? 'Every finished parcel in this range was delivered.'
                : 'No parcel in this range has finished yet.'}
            </EmptyState>
          }
        />
      </Section>

      <Section
        title="Daily parcels"
        description="Each count is placed on the day it happened: created, delivered or failed."
      >
        <DailyChart rows={daily} />
        <DataTable
          caption="Daily parcels"
          columns={dailyColumns}
          rows={daily}
          getRowId={(row) => row.date}
        />
      </Section>
    </>
  );
}