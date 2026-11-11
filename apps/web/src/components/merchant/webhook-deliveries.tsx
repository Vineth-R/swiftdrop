'use client';

import { CircleCheck, Clock, History, SquareX } from 'lucide-react';
import { useEffect, useState, type FormEvent } from 'react';

import { ApiError } from '../../lib/api';
import { cn } from '../../lib/cn';
import { formatDateTime } from '../../lib/format';
import {
  MAX_ATTEMPTS,
  attemptOutcome,
  listWebhookDeliveries,
  type AttemptOutcome,
  type DeliveryQuery,
  type WebhookDeliveryAttempt,
} from '../../lib/webhook-api';
import { Alert, alertToneClasses } from '../ui/alert';
import { Button } from '../ui/button';
import {
  DataTable,
  TablePagination,
  type DataTableColumn,
} from '../ui/data-table';
import { EmptyState } from '../ui/empty-state';
import { TextField } from '../ui/field';
import { Select } from '../ui/select';

const PAGE_SIZE = 20;

const noFilters = { success: '', trackingNumber: '' } as const;

const outcomes = {
  delivered: { label: 'Delivered', icon: CircleCheck, tone: 'success' },
  retrying: { label: 'Will retry', icon: Clock, tone: 'warning' },
  'gave-up': { label: 'Gave up', icon: SquareX, tone: 'danger' },
  failed: { label: 'Failed', icon: SquareX, tone: 'danger' },
} as const satisfies Record<
  AttemptOutcome,
  { label: string; icon: typeof Clock; tone: 'success' | 'warning' | 'danger' }
>;

/** A label and a shape for how an attempt ended, never colour alone. */
function ResultChip({ attempt }: { attempt: WebhookDeliveryAttempt }) {
  const { label, icon: Icon, tone } = outcomes[attemptOutcome(attempt)];
  return (
    <span
      className={cn(
        'inline-flex items-center gap-2 rounded-chip border px-2 py-1 text-overline uppercase',
        alertToneClasses[tone].box,
      )}
    >
      <Icon size={14} strokeWidth={1.5} aria-hidden="true" />
      {label}
    </span>
  );
}

const columns: DataTableColumn<WebhookDeliveryAttempt>[] = [
  {
    key: 'trackingNumber',
    header: 'Tracking number',
    isMono: true,
    cell: (row) => row.trackingNumber,
  },
  {
    key: 'attempt',
    header: 'Attempt',
    cell: (row) => `${row.attemptNumber} of ${MAX_ATTEMPTS}`,
  },
  {
    key: 'result',
    header: 'Result',
    cell: (row) => <ResultChip attempt={row} />,
  },
  {
    key: 'httpStatus',
    header: 'HTTP status',
    cell: (row) =>
      row.httpStatus === null ? (
        'No response'
      ) : (
        <span className="font-mono">{row.httpStatus}</span>
      ),
  },
  {
    key: 'sent',
    header: 'Sent',
    cell: (row) => (
      <span className="flex flex-col">
        <span>{formatDateTime(row.createdAt)}</span>
        {row.nextRetryAt && (
          <span className="text-caption text-text-muted">
            Next try {formatDateTime(row.nextRetryAt)}
          </span>
        )}
      </span>
    ),
  },
  {
    key: 'eventId',
    header: 'Event ID',
    isMono: true,
    cell: (row) => <span className="break-all">{row.eventId}</span>,
  },
];

type Load =
  | { status: 'loading' }
  | {
      status: 'ready';
      rows: WebhookDeliveryAttempt[];
      totalPages: number;
      totalItems: number;
    }
  | { status: 'error'; isNetwork: boolean };

/** Every delivery attempt, newest first, with filters for outcome and tracking number. */
export function WebhookDeliveries() {
  const [draft, setDraft] = useState<Omit<DeliveryQuery, 'page'>>(noFilters);
  const [query, setQuery] = useState<DeliveryQuery>({ page: 1, ...noFilters });
  const [load, setLoad] = useState<Load>({ status: 'loading' });
  const [revision, setRevision] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    setLoad({ status: 'loading' });
    listWebhookDeliveries(query, PAGE_SIZE, controller.signal)
      .then(({ data, meta }) =>
        setLoad({
          status: 'ready',
          rows: data,
          totalPages: meta.pagination?.totalPages ?? 1,
          totalItems: meta.pagination?.totalItems ?? data.length,
        }),
      )
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        setLoad({
          status: 'error',
          isNetwork: error instanceof ApiError && error.code === 'NETWORK',
        });
      });
    return () => controller.abort();
  }, [query, revision]);

  const isFiltered = query.success !== '' || query.trackingNumber !== '';

  function apply(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setQuery({
      page: 1,
      ...draft,
      trackingNumber: draft.trackingNumber.trim(),
    });
  }

  function clear() {
    setDraft(noFilters);
    setQuery({ page: 1, ...noFilters });
  }

  return (
    <div className="flex flex-col gap-4">
      <form
        noValidate
        onSubmit={apply}
        className="grid gap-4 rounded-card border border-line bg-surface p-5 sm:grid-cols-2 lg:grid-cols-[1fr_1fr_auto]"
      >
        <Select
          id="webhook-filter-result"
          label="Result"
          value={draft.success}
          options={[
            { value: '', label: 'All results' },
            { value: 'true', label: 'Delivered' },
            { value: 'false', label: 'Not delivered' },
          ]}
          onChange={(event) =>
            setDraft({
              ...draft,
              success: event.target.value as DeliveryQuery['success'],
            })
          }
        />
        <TextField
          id="webhook-filter-tracking"
          label="Tracking number"
          isMono
          maxLength={20}
          value={draft.trackingNumber}
          onChange={(event) =>
            setDraft({ ...draft, trackingNumber: event.target.value })
          }
        />
        <div className="flex items-end gap-3">
          <Button type="submit" variant="secondary">
            Apply filters
          </Button>
          {isFiltered && (
            <Button variant="ghost" onClick={clear}>
              Clear
            </Button>
          )}
        </div>
      </form>

      {load.status === 'error' ? (
        <Alert
          tone="danger"
          title="Could not load the delivery log"
          actions={
            <Button
              variant="secondary"
              onClick={() => setRevision((n) => n + 1)}
            >
              Try again
            </Button>
          }
        >
          {load.isNetwork
            ? 'We could not reach SwiftDrop. Check your connection and try again.'
            : 'Something broke on our side. Please try again in a moment.'}
        </Alert>
      ) : (
        <>
          <DataTable
            caption="Webhook delivery attempts"
            columns={columns}
            rows={load.status === 'ready' ? load.rows : []}
            getRowId={(row) => `${row.eventId}-${row.attemptNumber}`}
            isLoading={load.status === 'loading'}
            empty={
              <EmptyState
                title={isFiltered ? 'No attempts match' : 'No deliveries yet'}
                icon={History}
              >
                {isFiltered
                  ? 'Try different filters, or clear them to see everything.'
                  : 'Attempts appear here once one of your parcels changes status.'}
              </EmptyState>
            }
          />
          {load.status === 'ready' && (
            <TablePagination
              page={query.page}
              totalPages={load.totalPages}
              totalItems={load.totalItems}
              onPageChange={(page) => setQuery({ ...query, page })}
            />
          )}
        </>
      )}
    </div>
  );
}
