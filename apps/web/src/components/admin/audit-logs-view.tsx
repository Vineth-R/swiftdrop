'use client';

import { FileClock, RefreshCw } from 'lucide-react';
import { useEffect, useState } from 'react';

import { ApiError } from '@/lib/api';
import {
  listAuditLogs,
  type AuditLogEntry,
  type AuditLogPage,
  type AuditLogSource,
} from '@/lib/audit-logs-api';
import { formatDateTime } from '@/lib/format';
import { PageHeader } from '../layout/page-header';
import { Section } from '../layout/section';
import { Alert } from '../ui/alert';
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

const sources: { value: AuditLogSource; label: string }[] = [
  { value: 'identity', label: 'Identity' },
  { value: 'merchant', label: 'Merchant' },
  { value: 'pricing', label: 'Pricing' },
];

const emptyPage: AuditLogPage = {
  items: [],
  pagination: { page: 1, pageSize: PAGE_SIZE, totalItems: 0, totalPages: 0 },
};

function detailsText(details: Record<string, unknown>) {
  return JSON.stringify(details, null, 2);
}

const columns: DataTableColumn<AuditLogEntry>[] = [
  {
    key: 'createdAt',
    header: 'When',
    cell: (entry) => formatDateTime(entry.createdAt),
  },
  {
    key: 'action',
    header: 'Action',
    cell: (entry) => entry.action,
    isMono: true,
  },
  {
    key: 'entity',
    header: 'Resource',
    cell: (entry) => (
      <span className="font-mono text-caption">
        {entry.entityType}
        {entry.entityId ? ` · ${entry.entityId}` : ''}
      </span>
    ),
  },
  {
    key: 'actor',
    header: 'Actor',
    cell: (entry) => (
      <span className="font-mono text-caption">{entry.actorUserId}</span>
    ),
  },
  {
    key: 'details',
    header: 'Details',
    cell: (entry) => (
      <details className="max-w-sm">
        <summary className="cursor-pointer text-caption text-interactive underline underline-offset-2">
          Show details
        </summary>
        <pre className="mt-2 max-h-64 overflow-auto rounded-control bg-surface-2 p-2 font-mono text-caption">
          {detailsText(entry.details)}
        </pre>
      </details>
    ),
  },
];

export function AuditLogsView() {
  const [source, setSource] = useState<AuditLogSource>('identity');
  const [actor, setActor] = useState('');
  const [action, setAction] = useState('');
  const [entityType, setEntityType] = useState('');
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const [page, setPage] = useState(1);
  const [result, setResult] = useState<AuditLogPage>(emptyPage);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiError | null>(null);
  const [revision, setRevision] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError(null);
    listAuditLogs(source, {
      page,
      pageSize: PAGE_SIZE,
      actorUserId: actor.trim() || undefined,
      action: action.trim() || undefined,
      entityType: entityType.trim() || undefined,
      from: from || undefined,
      to: to || undefined,
      signal: controller.signal,
    })
      .then((next) => {
        if (!controller.signal.aborted) setResult(next);
      })
      .catch((caught: unknown) => {
        if (
          controller.signal.aborted ||
          (caught instanceof DOMException && caught.name === 'AbortError')
        ) {
          return;
        }
        setError(
          caught instanceof ApiError
            ? caught
            : new ApiError({ code: 'INTERNAL', status: 0 }),
        );
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [source, page, actor, action, entityType, from, to, revision]);

  function applyFilters(event: React.FormEvent) {
    event.preventDefault();
    setPage(1);
    setRevision((value) => value + 1);
  }

  function changeSource(next: string) {
    setSource(next as AuditLogSource);
    setPage(1);
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Audit logs"
        description="Read-only administrative actions recorded by the owning service. Credential-like detail fields are redacted by the API."
        actions={
          <Button
            variant="secondary"
            iconLeading={<RefreshCw size={16} />}
            onClick={() => setRevision((value) => value + 1)}
            isLoading={loading}
          >
            Refresh
          </Button>
        }
      />

      <Section title="Filters">
        <form
          className="grid gap-4 md:grid-cols-2 xl:grid-cols-6"
          onSubmit={applyFilters}
        >
          <Select
            id="audit-source"
            label="Source"
            value={source}
            options={sources}
            onChange={(event) => changeSource(event.target.value)}
          />
          <TextField
            id="audit-actor"
            label="Actor user ID"
            value={actor}
            onChange={(event) => setActor(event.target.value)}
            placeholder="UUID"
            isMono
          />
          <TextField
            id="audit-action"
            label="Action"
            value={action}
            onChange={(event) => setAction(event.target.value)}
            placeholder="USER_UPDATED"
          />
          <TextField
            id="audit-entity-type"
            label="Resource type"
            value={entityType}
            onChange={(event) => setEntityType(event.target.value)}
            placeholder="user"
          />
          <TextField
            id="audit-from"
            label="From"
            type="date"
            value={from}
            onChange={(event) => setFrom(event.target.value)}
          />
          <TextField
            id="audit-to"
            label="To"
            type="date"
            value={to}
            onChange={(event) => setTo(event.target.value)}
          />
          <div className="md:col-span-2 xl:col-span-6">
            <Button type="submit" variant="secondary">
              Apply filters
            </Button>
          </div>
        </form>
      </Section>

      <Section
        title="Recorded actions"
        description={`${result.pagination.totalItems} record(s) in the selected source.`}
      >
        {error ? (
          <Alert tone="danger" title="Could not load audit logs">
            {error.status === 403
              ? 'Administrator access is required.'
              : 'Try again. If the problem continues, check the service readiness.'}
          </Alert>
        ) : (
          <>
            <DataTable
              caption="Administrative audit logs"
              columns={columns}
              rows={result.items}
              getRowId={(entry) => entry.id}
              isLoading={loading}
              empty={
                <EmptyState icon={FileClock} title="No audit records">
                  No actions match the selected filters.
                </EmptyState>
              }
            />
            <TablePagination
              page={result.pagination.page}
              totalPages={result.pagination.totalPages}
              totalItems={result.pagination.totalItems}
              onPageChange={setPage}
              isLoading={loading}
            />
          </>
        )}
      </Section>
    </div>
  );
}
