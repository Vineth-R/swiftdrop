'use client';

import { useEffect, useState } from 'react';
import { ApiError } from '../../lib/api';
import { formatDateTime, formatKg } from '../../lib/format';
import {
  getParcel,
  getParcelHistory,
  getParcelProofPhoto,
  getParcelReceipt,
  type Parcel,
  type ParcelHistory,
} from '../../lib/parcels-api';
import { useSession } from '../../lib/session';
import { PageHeader } from '../layout/page-header';
import { RequireRole } from '../layout/require-role';
import { Section } from '../layout/section';
import { Alert } from '../ui/alert';
import { Button, ButtonLink } from '../ui/button';
import { EmptyState } from '../ui/empty-state';
import { KeyValueList } from '../ui/key-value-list';
import { SkeletonGroup, SkeletonText } from '../ui/skeleton';
import { StatusChip, statusLabel } from '../ui/status-chip';
import { Timeline } from '../ui/timeline';
import { ProofPhotoButton } from '../proof-photo-button';
import { FeeBreakdown } from './fee-breakdown';
import { ParcelProblem } from './parcel-problem';

export function ParcelDetailView({ id }: { id: string }) {
  return (
    <RequireRole
      roles={['CUSTOMER', 'DISPATCHER', 'ADMIN']}
      audience="customers and dispatchers"
    >
      <ParcelDetail id={id} />
    </RequireRole>
  );
}

type DetailState =
  | { status: 'loading' }
  | { status: 'ready'; parcel: Parcel; history: ParcelHistory[] }
  | { status: 'error'; error: unknown };

function ParcelDetail({ id }: { id: string }) {
  const { session } = useSession();
  const [state, setState] = useState<DetailState>({ status: 'loading' });
  const [retry, setRetry] = useState(0);
  const [receiptError, setReceiptError] = useState<unknown>(null);
  const [isDownloading, setIsDownloading] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    Promise.all([
      getParcel(id, controller.signal),
      getParcelHistory(id, controller.signal),
    ])
      .then(([parcel, history]) =>
        setState({ status: 'ready', parcel, history }),
      )
      .catch((error: unknown) => {
        if (!controller.signal.aborted) setState({ status: 'error', error });
      });
    return () => controller.abort();
  }, [id, retry]);

  async function download() {
    if (state.status !== 'ready' || isDownloading) return;
    setIsDownloading(true);
    setReceiptError(null);
    try {
      const blob = await getParcelReceipt(id);
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement('a');
      anchor.href = url;
      anchor.download = `receipt_${state.parcel.trackingNumber}.pdf`;
      document.body.append(anchor);
      anchor.click();
      anchor.remove();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (error) {
      setReceiptError(error);
    } finally {
      setIsDownloading(false);
    }
  }

  const parcel = state.status === 'ready' ? state.parcel : null;
  return (
    <>
      <PageHeader
        title="Parcel details"
        description="Your delivery information and status history."
        actions={
          <ButtonLink href="/parcels" variant="secondary">
            Back to parcels
          </ButtonLink>
        }
      />
      {state.status === 'loading' ? (
        <Section title="Delivery">
          <SkeletonGroup label="Loading parcel details">
            <SkeletonText />
          </SkeletonGroup>
        </Section>
      ) : state.status === 'error' ? (
        <Section title="Delivery">
          {state.error instanceof ApiError &&
          state.error.code === 'NOT_FOUND' ? (
            <EmptyState title="Parcel not found">
              Check the link or return to your parcels.
            </EmptyState>
          ) : (
            <>
              <ParcelProblem error={state.error} />
              <Button
                variant="secondary"
                onClick={() => {
                  setState({ status: 'loading' });
                  setRetry((n) => n + 1);
                }}
              >
                Try again
              </Button>
            </>
          )}
        </Section>
      ) : (
        parcel && (
          <>
            <Section title="Delivery">
              <div className="flex flex-wrap items-center gap-4">
                <p className="break-all font-mono text-h4">
                  {parcel.trackingNumber}
                </p>
                <StatusChip status={parcel.status} />
              </div>
              {parcel.failureReason && (
                <Alert tone="warning" title="Delivery could not be completed">
                  {parcel.failureReason}
                </Alert>
              )}
              <KeyValueList
                layout="inline"
                items={[
                  {
                    label: 'Created',
                    value: formatDateTime(parcel.createdAt),
                    isMono: true,
                  },
                  {
                    label: 'Last updated',
                    value: formatDateTime(parcel.updatedAt),
                    isMono: true,
                  },
                  {
                    label: 'Delivered',
                    value: parcel.deliveredAt
                      ? formatDateTime(parcel.deliveredAt)
                      : null,
                    isMono: true,
                  },
                  {
                    label: 'Weight',
                    value: formatKg(parcel.weightKg),
                    isMono: true,
                  },
                  { label: 'Size', value: parcel.sizeCategory },
                  { label: 'Description', value: parcel.description },
                  {
                    label: 'Courier',
                    value: parcel.courier
                      ? `${parcel.courier.fullName} · ${parcel.courier.phone}`
                      : 'Not assigned yet',
                  },
                ]}
              />
            </Section>
            <Section title="Pickup and destination">
              <div className="grid gap-8 md:grid-cols-2">
                {(['sender', 'recipient'] as const).map((kind) => (
                  <div key={kind} className="flex min-w-0 flex-col gap-4">
                    <h3 className="text-h4">
                      {kind === 'sender' ? 'Sender' : 'Recipient'}
                    </h3>
                    <KeyValueList
                      items={[
                        { label: 'Name', value: parcel[kind].name },
                        {
                          label: 'Phone',
                          value: parcel[kind].phone,
                          isMono: true,
                        },
                        { label: 'Address', value: parcel[kind].address },
                        { label: 'City', value: parcel[kind].city },
                      ]}
                    />
                  </div>
                ))}
              </div>
            </Section>
            <Section
              title="Saved delivery fee"
              description="This is the fee accepted when the delivery was created."
            >
              <FeeBreakdown fee={parcel.fee} />
              {receiptError !== null && <ParcelProblem error={receiptError} />}
              {session.status === 'authenticated' &&
                session.user.role !== 'DISPATCHER' && (
                  <div>
                    <Button
                      variant="secondary"
                      onClick={() => void download()}
                      isLoading={isDownloading}
                    >
                      {isDownloading
                        ? 'Preparing receipt'
                        : 'Download PDF receipt'}
                    </Button>
                  </div>
                )}
            </Section>
            {parcel.status === 'DELIVERED' && (
              <Section
                title="Delivery proof"
                description="Photos are private and use a short-lived link."
              >
                <ProofPhotoButton
                  load={(signal) => getParcelProofPhoto(id, signal)}
                />
              </Section>
            )}
            <Section title="Delivery timeline">
              {state.history.length === 0 ? (
                <EmptyState title="No status updates yet">
                  Your delivery updates will appear here.
                </EmptyState>
              ) : (
                <Timeline
                  label="Delivery history"
                  items={state.history.map((entry, index) => ({
                    id: `${entry.changedAt}-${index}`,
                    title: statusLabel(entry.status),
                    at: entry.changedAt,
                    detail:
                      entry.note || entry.failureReason ? (
                        <span className="break-words">
                          {[entry.note, entry.failureReason]
                            .filter(Boolean)
                            .join(' · ')}
                        </span>
                      ) : undefined,
                    isCurrent: index === state.history.length - 1,
                  }))}
                />
              )}
            </Section>
          </>
        )
      )}
    </>
  );
}
