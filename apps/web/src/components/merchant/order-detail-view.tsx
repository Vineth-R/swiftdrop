'use client';

import { Download } from 'lucide-react';
import { useEffect, useState } from 'react';

import { ApiError } from '@/lib/api';
import {
  getMerchantOrder,
  getMerchantOrderReceipt,
  type MerchantOrderDetail,
} from '@/lib/merchant-api';
import { PageHeader } from '../layout/page-header';
import { Section } from '../layout/section';
import { Alert } from '../ui/alert';
import { Button, ButtonLink } from '../ui/button';
import { StatusChip } from '../ui/status-chip';
import {
  ApprovalNotice,
  LoadState,
  displayDate,
  merchantError,
  useMerchant,
} from './shared';
import { money } from './orders-view';

export function MerchantOrderDetailView({ orderId }: { orderId: string }) {
  const profile = useMerchant();
  const approved =
    !profile.loading &&
    !profile.error &&
    profile.merchant?.status === 'APPROVED';
  const [order, setOrder] = useState<MerchantOrderDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [receiptLoading, setReceiptLoading] = useState(false);
  const [receiptError, setReceiptError] = useState('');
  const [revision, setRevision] = useState(0);

  useEffect(() => {
    if (!approved) {
      setOrder(null);
      setLoading(false);
      return;
    }
    const controller = new AbortController();
    setLoading(true);
    setError('');
    getMerchantOrder(orderId, controller.signal)
      .then((data) => {
        if (!controller.signal.aborted) setOrder(data);
      })
      .catch((err) => {
        if (!controller.signal.aborted) {
          setError(
            err instanceof ApiError && err.status === 404
              ? 'This order could not be found in your merchant account.'
              : merchantError(err),
          );
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [orderId, revision, approved]);

  async function downloadReceipt() {
    if (!order || receiptLoading) return;
    setReceiptLoading(true);
    setReceiptError('');
    try {
      const blob = await getMerchantOrderReceipt(order.trackingNumber);
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement('a');
      anchor.href = url;
      anchor.download = `SwiftDrop-order-${order.trackingNumber}.pdf`;
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (err) {
      setReceiptError(
        err instanceof ApiError && err.status === 404
          ? 'This receipt could not be found in your merchant account.'
          : merchantError(err),
      );
    } finally {
      setReceiptLoading(false);
    }
  }

  return (
    <>
      <PageHeader
        title="Order details"
        description="Delivery information, charges and status history."
        actions={
          <div className="flex flex-wrap gap-3">
            <Button
              variant="secondary"
              iconLeading={<Download size={16} aria-hidden="true" />}
              isLoading={receiptLoading}
              onClick={downloadReceipt}
              isDisabled={!order}
            >
              {receiptLoading ? 'Preparing receipt' : 'Download receipt'}
            </Button>
            <ButtonLink href="/merchant/orders" variant="secondary">
              Back to orders
            </ButtonLink>
          </div>
        }
      />
      {receiptError ? (
        <Alert tone="danger" title="Could not download the receipt">
          {receiptError}
        </Alert>
      ) : null}
      <LoadState {...profile} />
      {profile.merchant && !profile.loading && !profile.error ? (
        <ApprovalNotice merchant={profile.merchant} />
      ) : null}
      {approved || (loading && !error) ? (
        <LoadState
          loading={loading}
          error={error}
          retry={() => setRevision((n) => n + 1)}
        />
      ) : null}
      {approved && !loading && !error && order ? (
        <>
          <div className="flex flex-wrap items-center justify-between gap-4 rounded-card border border-line bg-surface p-6">
            <div>
              <h2 className="break-all font-display text-h3">
                {order.externalOrderId}
              </h2>
              <p className="mt-2 break-all font-mono text-body-sm text-text-muted">
                {order.trackingNumber}
              </p>
            </div>
            <StatusChip status={order.status} />
          </div>
          <div className="grid gap-8 lg:grid-cols-2">
            <Section title="Delivery details">
              <dl className="grid grid-cols-2 gap-x-6 gap-y-4 rounded-card border border-line bg-surface p-6">
                {[
                  ['Recipient', order.recipientName],
                  ['Destination', order.dropoffCity],
                  ['Weight', `${order.weightKg} kg`],
                  ['Size', order.sizeCategory],
                  ['Description', order.description || 'No description'],
                  ['Created', displayDate(order.createdAt)],
                  ['Updated', displayDate(order.updatedAt)],
                ].map(([label, value]) => (
                  <div key={label} className="min-w-0">
                    <dt className="text-caption text-text-muted">{label}</dt>
                    <dd className="mt-1 break-words text-body-sm">{value}</dd>
                  </div>
                ))}
              </dl>
            </Section>
            <Section title="Delivery fee">
              <dl className="flex flex-col gap-4 rounded-card border border-line bg-surface p-6">
                {[
                  ['Base fee', order.fee.baseFee],
                  ['Weight charge', order.fee.weightCharge],
                  ['Size surcharge', order.fee.sizeSurcharge],
                  ['Total', order.fee.total],
                ].map(([label, value]) => (
                  <div
                    key={String(label)}
                    className="flex flex-wrap justify-between gap-3"
                  >
                    <dt className="text-body-sm text-text-muted">{label}</dt>
                    <dd className="font-mono text-body-sm tabular-nums">
                      {money(Number(value), order.fee.currency)}
                    </dd>
                  </div>
                ))}
              </dl>
            </Section>
          </div>
          <Section title="Status timeline">
            {order.timeline.length ? (
              <ol className="flex flex-col gap-5 rounded-card border border-line bg-surface p-6">
                {[...order.timeline]
                  .sort((a, b) => Date.parse(a.at) - Date.parse(b.at))
                  .map((event, index) => (
                    <li
                      key={`${event.at}-${index}`}
                      className="flex flex-wrap items-center justify-between gap-3 border-l-2 border-primary pl-4"
                    >
                      <StatusChip status={event.status} />
                      <time
                        dateTime={event.at}
                        className="font-mono text-caption text-text-muted"
                      >
                        {new Date(event.at).toLocaleString('en-GB', {
                          timeZone: 'Asia/Colombo',
                        })}{' '}
                        (Sri Lanka)
                      </time>
                    </li>
                  ))}
              </ol>
            ) : (
              <p className="text-body-sm text-text-muted">
                No status updates yet.
              </p>
            )}
          </Section>
        </>
      ) : null}
    </>
  );
}
