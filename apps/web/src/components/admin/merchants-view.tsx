'use client';

import { useEffect, useState, type FormEvent } from 'react';

import { ApiError } from '@/lib/api';
import {
  approveMerchant,
  getAdminMerchant,
  listAdminMerchants,
  rejectMerchant,
  suspendMerchant,
  type Merchant,
} from '@/lib/merchant-api';
import { PageHeader } from '../layout/page-header';
import { Section } from '../layout/section';
import { Alert } from '../ui/alert';
import { Button } from '../ui/button';
import { DataTable, type DataTableColumn } from '../ui/data-table';
import { EmptyState } from '../ui/empty-state';
import { Select } from '../ui/select';
import { Modal } from '../ui/modal';
import { StatusChip } from '../ui/status-chip';
import { TextField } from '../ui/field';
import { Textarea } from '../ui/textarea';

type Action = {
  kind: 'approve' | 'reject' | 'suspend';
  merchant: Merchant;
} | null;

function displayDate(value?: string | null) {
  if (!value) return '—';
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? '—'
    : new Intl.DateTimeFormat('en-GB', {
        dateStyle: 'medium',
        timeStyle: 'short',
      }).format(date);
}

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
  const [selected, setSelected] = useState<Merchant | null>(null);
  const [action, setAction] = useState<Action>(null);
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState('');
  const [notice, setNotice] = useState('');
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

  async function openDetail(merchant: Merchant) {
    setSelected(merchant);
    try {
      const full = await getAdminMerchant(merchant.id);
      setSelected(full);
    } catch {
      // The row already carries enough detail for the modal.
    }
  }

  async function submitAction(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!action || busy) return;
    setBusy(true);
    setActionError('');
    try {
      const result =
        action.kind === 'approve'
          ? await approveMerchant(action.merchant.id)
          : action.kind === 'reject'
            ? await rejectMerchant(action.merchant.id, reason)
            : await suspendMerchant(action.merchant.id, reason);
      setNotice(
        result.identitySync === 'PENDING'
          ? 'The decision was saved. Account session invalidation is pending; retry from this screen when the identity service is available.'
          : action.kind === 'approve'
            ? 'Merchant approved and activated.'
            : action.kind === 'reject'
              ? 'Merchant rejected.'
              : 'Merchant suspended and sessions invalidated.',
      );
      setAction(null);
      setSelected(null);
      setReason('');
      setRevision((n) => n + 1);
    } catch (err) {
      setActionError(
        err instanceof ApiError && err.code === 'MERCHANT_ACTIVATION_FAILED'
          ? 'Activation failed. The merchant remains inactive. Retry safely.'
          : err instanceof ApiError && err.status === 422
            ? 'Check the reason and try again.'
            : 'The decision could not be saved. Try again.',
      );
    } finally {
      setBusy(false);
    }
  }

  const columns: DataTableColumn<Merchant>[] = [
    {
      key: 'businessName',
      header: 'Business',
      cell: (row) => (
        <button
          type="button"
          className="text-left font-semibold text-interactive underline underline-offset-4"
          onClick={() => openDetail(row)}
        >
          {row.businessName}
        </button>
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
      cell: (row) => <StatusChip status={row.status} />,
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
        <div className="flex flex-wrap gap-2">
          <Button variant="ghost" size="sm" onClick={() => openDetail(row)}>
            Review
          </Button>
          {row.status === 'PENDING' || row.status === 'SUSPENDED' ? (
            <Button
              size="sm"
              onClick={() => {
                setActionError('');
                setAction({ kind: 'approve', merchant: row });
              }}
            >
              Approve
            </Button>
          ) : null}
          {row.status === 'PENDING' ? (
            <Button
              variant="danger"
              size="sm"
              onClick={() => {
                setActionError('');
                setAction({ kind: 'reject', merchant: row });
              }}
            >
              Reject
            </Button>
          ) : null}
          {row.status === 'APPROVED' ? (
            <Button
              variant="secondary"
              size="sm"
              onClick={() => {
                setActionError('');
                setAction({ kind: 'suspend', merchant: row });
              }}
            >
              Suspend
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
      {notice ? <Alert tone="success" title={notice} /> : null}
      {error ? (
        <Alert tone="danger" title="Could not load merchants">
          {error}
        </Alert>
      ) : null}
      <Section title="Applications">
        <form
          className="flex flex-wrap items-end gap-4"
          onSubmit={(event) => {
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
          empty={<EmptyState title="No merchant applications" />}
        />
        {pages > 1 ? (
          <div className="flex items-center justify-between gap-4">
            <p className="text-caption text-text-muted">
              Page {page} of {pages}
            </p>
            <div className="flex gap-2">
              <Button
                variant="secondary"
                size="sm"
                isDisabled={page <= 1 || loading}
                onClick={() => setPage((value) => Math.max(1, value - 1))}
              >
                Previous
              </Button>
              <Button
                variant="secondary"
                size="sm"
                isDisabled={page >= pages || loading}
                onClick={() => setPage((value) => value + 1)}
              >
                Next
              </Button>
            </div>
          </div>
        ) : null}
      </Section>

      <Modal
        isOpen={!!selected}
        onClose={() => setSelected(null)}
        title={selected?.businessName ?? 'Merchant'}
        description="Application details and current access state."
        actions={
          <Button variant="secondary" onClick={() => setSelected(null)}>
            Close
          </Button>
        }
      >
        {selected ? (
          <dl className="grid gap-3 text-body-sm sm:grid-cols-2">
            <div>
              <dt className="text-text-muted">Status</dt>
              <dd className="mt-1">
                <StatusChip status={selected.status} />
              </dd>
            </div>
            <div>
              <dt className="text-text-muted">Registration</dt>
              <dd className="mt-1 font-mono">
                {selected.registrationNumber ?? '—'}
              </dd>
            </div>
            <div>
              <dt className="text-text-muted">Owner user</dt>
              <dd className="mt-1 break-all font-mono">
                {selected.ownerUserId}
              </dd>
            </div>
            <div>
              <dt className="text-text-muted">Submitted</dt>
              <dd className="mt-1">{displayDate(selected.createdAt)}</dd>
            </div>
            <div>
              <dt className="text-text-muted">Business email</dt>
              <dd className="mt-1 break-all">{selected.contactEmail}</dd>
            </div>
            <div>
              <dt className="text-text-muted">Business phone</dt>
              <dd className="mt-1 font-mono">{selected.contactPhone}</dd>
            </div>
            <div className="sm:col-span-2">
              <dt className="text-text-muted">Business address</dt>
              <dd className="mt-1">{selected.businessAddress ?? '—'}</dd>
            </div>
            <div className="sm:col-span-2">
              <dt className="text-text-muted">Default pickup</dt>
              <dd className="mt-1">
                {selected.pickupAddress} · zone {selected.pickupZoneId}
              </dd>
            </div>
            <div className="sm:col-span-2">
              <dt className="text-text-muted">Rejection reason</dt>
              <dd className="mt-1">
                {selected.rejectionReason ?? 'No reason provided'}
              </dd>
            </div>
            <div className="sm:col-span-2">
              <dt className="text-text-muted">Suspension reason</dt>
              <dd className="mt-1">{selected.suspensionReason ?? '—'}</dd>
            </div>
          </dl>
        ) : null}
      </Modal>

      <Modal
        isOpen={!!action}
        onClose={() => setAction(null)}
        title={
          action?.kind === 'approve'
            ? 'Approve application?'
            : action?.kind === 'reject'
              ? 'Reject application?'
              : 'Suspend merchant?'
        }
        description={
          action?.kind === 'approve'
            ? 'The account is activated only after Identity applies the lifecycle transition.'
            : action?.kind === 'reject'
              ? 'The merchant stays inactive. A reason is optional.'
              : 'Merchant business operations stop immediately and sessions are invalidated.'
        }
        actions={
          <>
            <Button
              variant="secondary"
              isDisabled={busy}
              onClick={() => setAction(null)}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              form="merchant-action"
              variant={action?.kind === 'approve' ? 'primary' : 'danger'}
              isLoading={busy}
            >
              {action?.kind === 'approve'
                ? 'Approve'
                : action?.kind === 'reject'
                  ? 'Reject'
                  : 'Suspend'}
            </Button>
          </>
        }
      >
        <form
          id="merchant-action"
          onSubmit={submitAction}
          className="flex flex-col gap-4"
        >
          {actionError ? (
            <Alert tone="danger" title="Could not complete the decision">
              {actionError}
            </Alert>
          ) : null}
          {action?.kind !== 'approve' ? (
            <Textarea
              id="merchant-decision-reason"
              label="Reason (optional)"
              maxLength={1000}
              showCount
              rows={4}
              value={reason}
              onChange={(event) => setReason(event.target.value)}
              helper={
                action?.kind === 'reject'
                  ? 'This reason is included in the merchant email. Leave it blank to send a neutral rejection.'
                  : 'This reason is included in the merchant email.'
              }
            />
          ) : (
            <p className="text-body-sm text-text-muted">
              Approval activates the merchant account. If activation fails, the
              application stays pending and you can retry.
            </p>
          )}
        </form>
      </Modal>
    </>
  );
}
