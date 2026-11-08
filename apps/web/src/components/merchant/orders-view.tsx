'use client';

import Link from 'next/link';
import { Boxes } from 'lucide-react';
import { useEffect, useState, type FormEvent } from 'react';
import { listMerchantOrders, type MerchantOrder, type OrderQuery } from '@/lib/merchant-api';
import { PageHeader } from '../layout/page-header';
import { Section } from '../layout/section';
import { Alert } from '../ui/alert';
import { Button } from '../ui/button';
import { EmptyState } from '../ui/empty-state';
import { TextField } from '../ui/field';
import { Select } from '../ui/select';
import { StatusChip } from '../ui/status-chip';
import { ApprovalNotice, LoadState, displayDate, merchantError, useMerchant } from './shared';

const statuses = ['CREATED', 'PICKED_UP', 'IN_TRANSIT', 'OUT_FOR_DELIVERY', 'DELIVERED', 'FAILED'];
export function money(amount: number, currency: string) {
  return new Intl.NumberFormat('en-LK', { style: 'currency', currency }).format(amount);
}

export function MerchantOrdersView() {
  const profile = useMerchant();
  const [query, setQuery] = useState<OrderQuery>({ page: 1 });
  const [orders, setOrders] = useState<MerchantOrder[]>([]);
  const [pages, setPages] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [validation, setValidation] = useState('');
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const controller = new AbortController(); setLoading(true); setError('');
    listMerchantOrders(query, controller.signal).then((result) => {
      if (controller.signal.aborted) return;
      setOrders(result.data); setPages(result.meta.pagination?.totalPages || 1);
    }).catch((err) => { if (!controller.signal.aborted) setError(merchantError(err)); }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [query, revision]);
  function filter(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); const data = new FormData(event.currentTarget);
    const from = String(data.get('from') || ''); const to = String(data.get('to') || '');
    if (from && to && from > to) { setValidation('The end date must be on or after the start date.'); return; }
    setValidation(''); setQuery({ page: 1, status: String(data.get('status') || ''), from, to, externalOrderId: String(data.get('externalOrderId') || '').trim() });
  }
  return <><PageHeader title="Orders" description="Track delivery orders sent from your store." /><LoadState {...profile} />{profile.merchant && !profile.loading && !profile.error ? <ApprovalNotice merchant={profile.merchant} /> : null}
    <Section title="Find an order"><form onSubmit={filter} className="grid gap-4 rounded-card border border-line bg-surface p-5 sm:grid-cols-2 xl:grid-cols-5">
      <TextField id="merchant-order-search" name="externalOrderId" label="External order ID" placeholder="Search your order reference" maxLength={100} />
      <Select id="merchant-order-status" name="status" label="Status" options={[{ value: '', label: 'All statuses' }, ...statuses.map((value) => ({ value, label: value.replaceAll('_', ' ') }))]} />
      <TextField id="merchant-orders-from" name="from" label="From date" type="date" /><TextField id="merchant-orders-to" name="to" label="To date" type="date" />
      <div className="flex items-end"><Button type="submit" variant="secondary" isFullWidth>Apply filters</Button></div>
    </form>{validation ? <Alert tone="danger" title={validation} /> : null}</Section>
    <Section title="Delivery orders"><LoadState loading={loading} error={error} retry={() => setRevision((n) => n + 1)} />
      {!loading && !error ? orders.length ? <>
        <ul className="divide-y divide-line overflow-hidden rounded-card border border-line bg-surface">{orders.map((order) => <li key={order.orderId} className="grid gap-4 p-5 md:grid-cols-[1.4fr_1fr_1fr_auto] md:items-center">
          <div className="min-w-0"><Link className="break-all text-label text-interactive underline underline-offset-4" href={`/merchant/orders/${encodeURIComponent(order.orderId)}`}>{order.externalOrderId}</Link><p className="mt-1 break-all font-mono text-caption text-text-muted">{order.trackingNumber}</p></div>
          <div><p className="break-words text-body-sm">{order.recipientName}</p><p className="text-caption text-text-muted">{order.dropoffCity}</p></div><StatusChip status={order.status} /><div className="md:text-right"><p className="font-mono text-body-sm tabular-nums">{money(order.total, order.currency)}</p><p className="mt-1 text-caption text-text-muted">{displayDate(order.createdAt)}</p></div>
        </li>)}</ul>
        <nav aria-label="Order pagination" className="flex flex-wrap items-center justify-between gap-3"><Button variant="secondary" isDisabled={query.page <= 1} onClick={() => setQuery((old) => ({ ...old, page: old.page - 1 }))}>Previous</Button><span className="text-body-sm text-text-muted" role="status">Page {query.page} of {pages}</span><Button variant="secondary" isDisabled={query.page >= pages} onClick={() => setQuery((old) => ({ ...old, page: old.page + 1 }))}>Next</Button></nav>
      </> : <EmptyState title="No orders found" icon={Boxes}>Try different filters. Orders created through your store’s integration appear here.</EmptyState> : null}
    </Section></>;
}
