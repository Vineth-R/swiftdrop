'use client';

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type FormEvent,
} from 'react';

import { ApiError } from '../../lib/api';
import { RETRY_AFTER_FALLBACK_SECONDS } from '../../lib/constants';
import {
  MAX_RANGE_DAYS,
  defaultRange,
  formatReportDate,
  getDailyReport,
  getOutcomesReport,
  rangeErrorsFromIssues,
  validateRange,
  type DailyRow,
  type DateRange,
  type OutcomesReport,
  type RangeErrors,
} from '../../lib/reports-api';
import { describeRole } from '../../lib/roles';
import { useSession } from '../../lib/session';
import { useCountdown } from '../../lib/use-countdown';
import { useForm, type FormRules } from '../../lib/use-form';
import { RateLimitedAlert, ServerFaultAlert } from '../auth/form-alerts';
import { PageHeader } from '../layout/page-header';
import { RequireRole } from '../layout/require-role';
import { Section } from '../layout/section';
import { SystemPage } from '../layout/system-page';
import { Button, ButtonLink } from '../ui/button';
import { Card, CardContent } from '../ui/card';
import {
  DataTable,
  TablePagination,
  type DataTableColumn,
} from '../ui/data-table';
import { EmptyState } from '../ui/empty-state';
import { TextField } from '../ui/field';
import { KeyValueList } from '../ui/key-value-list';
import { Skeleton, SkeletonGroup } from '../ui/skeleton';
import { DailyChart } from './daily-chart';

type Loaded = { daily: DailyRow[]; outcomes: OutcomesReport };

type State =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'error'; error: ApiError }
  | ({ status: 'ready' } & Loaded);

const INTRO = 'Parcel volumes and delivery outcomes for the dates you choose.';
const PRESET_DAYS = [7, 14, 30, 90] as const;
const DAILY_PAGE_SIZE = 15;
const rangeRules: FormRules<DateRange> = {
  from: (values) => validateRange(values).from,
  to: (values) => validateRange(values).to,
};

/**
 * /reports: parcel volumes and delivery outcomes. Dispatchers and
 * administrators, matching API v1.1 and the existing Dispatch navigation. The
 * task sheet's ADMIN-only wording is not enforced by the published API.
 */
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
  const {
    values,
    errors,
    setValue,
    blur,
    validate,
    setServerErrors,
    resetValidation,
  } = useForm<DateRange>(range, rangeRules);
  const [attempt, setAttempt] = useState(0);
  const [state, setState] = useState<State>({ status: 'loading' });
  const focusTarget = useRef<'from' | 'to' | null>(null);
  const [focusRequest, setFocusRequest] = useState(0);

  // Focus after the error message has rendered, so the field is still mounted.
  const requestFocus = useCallback((problems: RangeErrors) => {
    focusTarget.current = problems.from ? 'from' : problems.to ? 'to' : null;
    if (focusTarget.current) setFocusRequest((count) => count + 1);
  }, []);

  useEffect(() => {
    const target = focusTarget.current;
    if (!target) return;
    const element = document.getElementById(
      target === 'from' ? 'reports-from' : 'reports-to',
    );
    element?.focus();
  }, [focusRequest]);

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
        if (!(error instanceof ApiError)) {
          setState({
            status: 'error',
            error: new ApiError({ code: 'INTERNAL', status: 0 }),
          });
          return;
        }
        if (error.code === 'UNAUTHENTICATED') {
          // The session gate is already sending the person to the login page.
          setState({ status: 'idle' });
          return;
        }
        if (error.code === 'VALIDATION_FAILED') {
          const problems = rangeErrorsFromIssues(error.issues);
          setServerErrors(problems);
          setState({ status: 'idle' });
          requestFocus(problems);
          return;
        }
        setState({ status: 'error', error });
      });
    return () => controller.abort();
  }, [range, attempt, setServerErrors, requestFocus]);

  function load(next: DateRange) {
    setState({ status: 'loading' });
    // A new object every time, so applying the same dates again reloads.
    setRange({ ...next });
  }

  function apply(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (state.status === 'loading') return;
    const problems = validate();
    if (Object.keys(problems).length > 0) {
      requestFocus(problems);
      return;
    }
    load(values);
  }

  function retry() {
    setState({ status: 'loading' });
    setAttempt((count) => count + 1);
  }

  function reset() {
    const initial = defaultRange();
    setValue('from', initial.from);
    setValue('to', initial.to);
    resetValidation();
    load(initial);
  }

  function choosePreset(days: number) {
    const next = defaultRange(days);
    setValue('from', next.from);
    setValue('to', next.to);
    resetValidation();
    load(next);
  }

  // A server-side 403 means this person may not read reports, even if the
  // session role says otherwise. Show the same no-permission state as
  // RequireRole and do not offer another request.
  if (state.status === 'error' && state.error.code === 'FORBIDDEN') {
    return <ReportsForbidden />;
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
            controlSize="lg"
            value={values.from}
            max={values.to || undefined}
            error={errors.from}
            onChange={(event) => setValue('from', event.target.value)}
            onBlur={() => blur('from')}
          />
          <TextField
            id="reports-to"
            label="To"
            type="date"
            controlSize="lg"
            value={values.to}
            min={values.from || undefined}
            error={errors.to}
            onChange={(event) => setValue('to', event.target.value)}
            onBlur={() => blur('to')}
          />
          <Button
            type="submit"
            size="lg"
            className="sm:mt-6"
            isLoading={state.status === 'loading'}
          >
            {state.status === 'loading' ? 'Loading' : 'Apply range'}
          </Button>
        </form>
        <div
          role="group"
          aria-label="Quick ranges"
          className="flex flex-wrap gap-2"
        >
          {PRESET_DAYS.map((days) => (
            <Button
              key={days}
              variant="secondary"
              size="lg"
              isDisabled={state.status === 'loading'}
              onClick={() => choosePreset(days)}
            >
              Last {days} days
            </Button>
          ))}
        </div>
      </Section>

      {state.status === 'loading' && <ReportsSkeleton />}
      {state.status === 'error' && (
        <ReportsError error={state.error} onRetry={retry} />
      )}
      {state.status === 'ready' && isEmpty(state) && (
        <EmptyState
          title="No parcels in this range"
          action={
            <Button size="lg" onClick={reset}>
              Reset range
            </Button>
          }
        >
          Nothing was created, delivered or failed between{' '}
          {formatReportDate(range.from)} and {formatReportDate(range.to)}. Try a
          wider range.
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

function ReportsForbidden() {
  const { session } = useSession();
  const role =
    session.status === 'authenticated' ? session.user.role : undefined;

  return (
    <SystemPage
      label="403 · Forbidden"
      title="You do not have access"
      actions={
        <ButtonLink href="/dashboard" variant="secondary" size="lg">
          Back to my dashboard
        </ButtonLink>
      }
    >
      This page is for dispatchers and administrators.
      {role
        ? ` You are signed in as ${describeRole(role)}, which is probably correct: nothing is wrong with your account.`
        : ''}
    </SystemPage>
  );
}

function ReportsError({
  error,
  onRetry,
}: {
  error: ApiError;
  onRetry: () => void;
}) {
  const seconds =
    error.code === 'RATE_LIMITED'
      ? (error.retryAfter ?? RETRY_AFTER_FALLBACK_SECONDS)
      : null;
  const remaining = useCountdown(seconds);

  if (error.code === 'RATE_LIMITED') {
    return <RateLimitedAlert remaining={remaining} />;
  }

  if (error.isServerFault) {
    return (
      <div className="flex flex-col items-start gap-3">
        <ServerFaultAlert
          isNetwork={error.code === 'NETWORK'}
          requestId={error.code === 'INTERNAL' ? error.requestId : undefined}
        />
        <Button variant="secondary" size="lg" onClick={onRetry}>
          Try again
        </Button>
      </div>
    );
  }

  // An unexpected failure still gets our own wording, never the server's.
  return <ServerFaultAlert isNetwork={false} />;
}

const dailyColumns: DataTableColumn<DailyRow>[] = [
  {
    key: 'date',
    header: 'Date',
    cell: (row) => formatReportDate(row.date),
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
  return `${Number(value.toFixed(2))}%`;
}

function ReportsReady({ daily, outcomes }: Loaded) {
  const { totals } = outcomes;
  const [page, setPage] = useState(1);
  const totalPages = Math.max(1, Math.ceil(daily.length / DAILY_PAGE_SIZE));
  const newestFirst = [...daily].sort((a, b) => b.date.localeCompare(a.date));
  const visibleDays = newestFirst.slice(
    (page - 1) * DAILY_PAGE_SIZE,
    page * DAILY_PAGE_SIZE,
  );
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
        description="Delivered and failed parcels, each placed on the day it happened. Created counts are in the table below."
      >
        <DailyChart rows={daily} />
        <DataTable
          caption="Daily parcels"
          columns={dailyColumns}
          rows={visibleDays}
          getRowId={(row) => row.date}
        />
        <TablePagination
          page={page}
          totalPages={totalPages}
          totalItems={daily.length}
          onPageChange={setPage}
        />
      </Section>
    </>
  );
}
