'use client';

import { useEffect, useState, type FormEvent } from 'react';

import { ApiError } from '@/lib/api';
import { formatDateTime } from '@/lib/format';
import {
  listAdminMerchants,
  rejectMerchant,
  suspendMerchant,
  type Merchant,
} from '@/lib/merchant-api';
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
import { Select } from '../ui/select';
import { StatusChip } from '../ui/status-chip';
import { TextField } from '../ui/field';
import { MerchantDetailsModal } from './merchant-details-modal';

/** A time in Sri Lanka time, or a dash when there is none. */
function displayDate(value?: string | null) {
  return (value && formatDateTime(value)) || '—';
}

type Notice = { tone: 'success' | 'warning'; text: string } | null;

export function MerchantsView() {
  const [rows, setRows] = useState<Merchant[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [status, setStatus] = useState('');
  const [search, setSearch] = useState('');
  const [query, setQuery] = useState('');
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [revision, setRevision] = useState(0);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [retrying, setRetrying] = useState<string | null>(null);
  const [notice, setNotice] = useState<Notice>(null);
  const pageSize = 20;

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError('');
    listAdminMerchants(
      { status: status || undefined, q: query || undefined, page, pageSize },
      controller.signal,
    )
      .then((result) => {
        if (controller.signal.aborted) return;
        setRows(result.data);
        setTotal(result.meta.pagination?.totalItems ?? result.data.length);
      })
      .catch((err) => {
        if (!controller.signal.aborted) {
          setError(
            err instanceof ApiError && err.status === 403
              ? 'Administrator access is required.'
              : 'Could not load merchant applications.',
          );
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [status, query, page, revision]);

  /**
   * Repeats a rejection or suspension whose status has not reached Identity.
   * The API replays the same transition, so this is safe to press twice, and it
   * does not email the merchant again. A suspension needs its reason, so the
   * stored one is sent with it.
   */
  async function retrySync(merchant: Merchant) {
    if (retrying) return;
    setRetrying(merchant.id);
    setNotice(null);
    setError('');
    try {
      const kind = merchant.status === 'REJECTED' ? 'reject' : 'suspend';
      const result =
        kind === 'reject'
          ? await rejectMerchant(merchant.id)
          : await suspendMerchant(merchant.id, merchant.suspensionReason);
      setNotice(
        result.identitySync === 'PENDING'
          ? {
              tone: 'warning',
              text: 'Identity could not be reached again. The account may still be signed in. Try again in a moment.',
            }
          : {
              tone: 'success',
              text: 'Identity applied the decision. The merchant is signed out everywhere.',
            },
      );
      setRevision((n) => n + 1);
    } catch {
      setError('The sync could not be retried. Try again in a moment.');
    } finally {
      setRetrying(null);
    }
  }

  function updateRow(updated: Merchant) {
    setRows((current) =>
      current.map((row) =>
        row.id === updated.id ? { ...row, ...updated } : row,
      ),
    );
    setRevision((n) => n + 1);
  }

  const columns: DataTableColumn<Merchant>[] = [
    {
      key: 'businessName',
      header: 'Business',
      cell: (row) => (
        <span className="font-semibold text-text">{row.businessName}</span>
      ),
    },
    {
      key: 'registrationNumber',
      header: 'Registration',
      cell: (row) => row.registrationNumber ?? '—',
      isMono: true,
    },
    { key: 'contactEmail', header: 'Contact', cell: (row) => row.contactEmail },
    {
      key: 'status',
      header: 'Status',
      cell: (row) => (
        <div className="flex flex-col items-start gap-1">
          <StatusChip status={row.status} />
          {row.identitySync === 'PENDING' ? (
            <span className="text-caption text-warning">
              Sign-out pending at Identity
            </span>
          ) : null}
        </div>
      ),
    },
    {
      key: 'createdAt',
      header: 'Submitted',
      cell: (row) => displayDate(row.createdAt),
    },
    {
      key: 'actions',
      header: 'Actions',
      isHeaderHidden: true,
      cell: (row) => (
        <div
          data-row-action
          className="flex flex-nowrap items-center gap-2"
          onClick={(event) => event.stopPropagation()}
        >
          <Button
            variant="secondary"
            size="sm"
            onClick={() => setSelectedId(row.id)}
            aria-label={`Review ${row.businessName}`}
          >
            Review
          </Button>
          {row.identitySync === 'PENDING' ? (
            <Button
              variant="secondary"
              size="sm"
              isLoading={retrying === row.id}
              isDisabled={retrying !== null}
              onClick={() => retrySync(row)}
            >
              Retry sync
            </Button>
          ) : null}
        </div>
      ),
    },
  ];

  const pages = Math.max(1, Math.ceil(total / pageSize));

  return (
    <>
      <PageHeader
        title="Merchants"
        description="Review business applications and manage account access."
      />
      {notice ? <Alert tone={notice.tone} title={notice.text} /> : null}
      {error ? (
        <Alert tone="danger" title="Could not load merchants">
          {error}
        </Alert>
      ) : null}
      <Section title="Applications">
        <form
          className="flex flex-wrap items-end gap-4"
          onSubmit={(event: FormEvent<HTMLFormElement>) => {
            event.preventDefault();
            setPage(1);
            setQuery(search.trim());
          }}
        >
          <div className="w-full max-w-70">
            <TextField
              id="merchant-search"
              label="Search"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Business name, registration number or email"
            />
          </div>
          <div className="w-full max-w-45">
            <Select
              id="merchant-status"
              label="Status"
              value={status}
              onChange={(event) => {
                setStatus(event.target.value);
                setPage(1);
              }}
              options={[
                { value: '', label: 'All statuses' },
                { value: 'PENDING', label: 'Pending' },
                { value: 'APPROVED', label: 'Approved' },
                { value: 'REJECTED', label: 'Rejected' },
                { value: 'SUSPENDED', label: 'Suspended' },
              ]}
            />
          </div>
          <Button type="submit" variant="secondary">
            Filter
          </Button>
        </form>
        <DataTable
          caption="Merchant applications"
          columns={columns}
          rows={rows}
          getRowId={(row) => row.id}
          isLoading={loading}
          onRowClick={(row) => setSelectedId(row.id)}
          rowLabel={(row) => row.businessName}
          empty={<EmptyState title="No merchant applications" />}
        />
        <TablePagination
          page={page}
          totalPages={pages}
          totalItems={total}
          onPageChange={setPage}
          isLoading={loading}
        />
      </Section>

      <MerchantDetailsModal
        merchantId={selectedId}
        onClose={() => setSelectedId(null)}
        onChanged={updateRow}
      />
    </>
  );
}
