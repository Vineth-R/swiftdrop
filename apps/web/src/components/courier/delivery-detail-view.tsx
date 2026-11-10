'use client';

import { useEffect, useState } from 'react';
import {
  getCourierDelivery,
  updateCourierDeliveryStatus,
  type CourierDeliveryDetail,
} from '../../lib/courier-api';
import { ApiError } from '../../lib/api';
import { PageHeader } from '../layout/page-header';
import { Section } from '../layout/section';
import { Button, ButtonLink } from '../ui/button';
import { Alert } from '../ui/alert';
import { StatusChip } from '../ui/status-chip';
import { TextField } from '../ui/field';

const nextStatuses: Record<string, string[]> = {
  CREATED: ['PICKED_UP', 'FAILED'],
  PICKED_UP: ['IN_TRANSIT', 'FAILED'],
  IN_TRANSIT: ['OUT_FOR_DELIVERY', 'FAILED'],
  OUT_FOR_DELIVERY: ['DELIVERED', 'FAILED'],
};

export function DeliveryDetailView({ parcelId }: { parcelId: string }) {
  const [delivery, setDelivery] = useState<CourierDeliveryDetail | null>(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [recipientName, setRecipientName] = useState('');
  const [failureReason, setFailureReason] = useState('');
  const [note, setNote] = useState('');
  const load = () => {
    setLoading(true);
    setError('');
    const controller = new AbortController();
    getCourierDelivery(parcelId, controller.signal)
      .then(setDelivery)
      .catch((e) =>
        setError(
          e instanceof ApiError && e.status === 403
            ? 'This delivery is not assigned to you.'
            : 'We could not load this delivery. Try again.',
        ),
      )
      .finally(() => setLoading(false));
    return () => controller.abort();
  };
  useEffect(load, [parcelId]);
  async function advance(status: string) {
    setBusy(true);
    setError('');
    try {
      await updateCourierDeliveryStatus(parcelId, {
        status: status as never,
        recipientName: recipientName.trim() || undefined,
        failureReason: failureReason.trim() || undefined,
        note: note.trim() || undefined,
      });
      setRecipientName('');
      setFailureReason('');
      setNote('');
      load();
    } catch (e) {
      setError(
        e instanceof ApiError && e.status === 422
          ? 'Add the required delivery details and try again.'
          : e instanceof ApiError && e.status === 409
            ? 'This delivery changed. Refresh and choose the next available action.'
            : 'We could not update this delivery. Try again.',
      );
    } finally {
      setBusy(false);
    }
  }
  if (loading)
    return (
      <p role="status" className="text-body text-text-muted">
        Loading delivery…
      </p>
    );
  if (error && !delivery)
    return (
      <Alert
        tone="danger"
        title="Could not load delivery"
        actions={
          <Button variant="secondary" onClick={load}>
            Try again
          </Button>
        }
      >
        {error}
      </Alert>
    );
  if (!delivery) return null;
  const options = nextStatuses[delivery.status] || [];
  return (
    <>
      <PageHeader
        title="Delivery details"
        description="Review the delivery and record its next status."
        actions={
          <ButtonLink href="/history" variant="secondary">
            Back to history
          </ButtonLink>
        }
      />
      {error ? (
        <Alert tone="danger" title="Could not update delivery">
          {error}
        </Alert>
      ) : null}
      <div className="flex flex-wrap items-center justify-between gap-4 rounded-card border border-line bg-surface p-6">
        <div>
          <p className="font-mono text-body-sm text-text-muted">
            {delivery.trackingNumber}
          </p>
          <h2 className="mt-2 font-display text-h3">
            {delivery.recipientName || 'Recipient unavailable'}
          </h2>
          <p className="mt-1 text-body-sm text-text-muted">
            {delivery.dropoffAddress}, {delivery.dropoffCity}
          </p>
        </div>
        <StatusChip status={delivery.status} />
      </div>
      <div className="grid gap-8 lg:grid-cols-2">
        <Section title="Delivery information">
          <dl className="grid gap-4 rounded-card border border-line bg-surface p-6">
            <div>
              <dt className="text-caption text-text-muted">Pickup</dt>
              <dd className="text-body-sm">{delivery.pickupAddress}</dd>
            </div>
            <div>
              <dt className="text-caption text-text-muted">Sender</dt>
              <dd className="text-body-sm">
                {delivery.senderName} · {delivery.senderPhone}
              </dd>
            </div>
            <div>
              <dt className="text-caption text-text-muted">Package</dt>
              <dd className="text-body-sm">
                {delivery.weightKg} kg · {delivery.sizeCategory}
              </dd>
            </div>
          </dl>
        </Section>
        <Section title="Update status">
          <div className="flex flex-col gap-4 rounded-card border border-line bg-surface p-6">
            {options.includes('DELIVERED') ? (
              <TextField
                id="recipient-name"
                label="Recipient name"
                value={recipientName}
                onChange={(e) => setRecipientName(e.target.value)}
                required
              />
            ) : null}
            {options.includes('FAILED') ? (
              <TextField
                id="failure-reason"
                label="Failure reason"
                value={failureReason}
                onChange={(e) => setFailureReason(e.target.value)}
              />
            ) : null}
            <TextField
              id="status-note"
              label="Note (optional)"
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />
            {options.map((status) => (
              <Button
                key={status}
                isDisabled={busy}
                isLoading={busy}
                variant={status === 'FAILED' ? 'danger' : 'primary'}
                onClick={() => advance(status)}
              >
                {status.replaceAll('_', ' ')}
              </Button>
            ))}
            {!options.length ? (
              <p className="text-body-sm text-text-muted">
                This delivery is complete.
              </p>
            ) : null}
          </div>
        </Section>
      </div>
      <Section title="Status history">
        <ol className="flex flex-col gap-4 rounded-card border border-line bg-surface p-6">
          {delivery.history.map((event) => (
            <li
              key={event.changedAt}
              className="flex flex-wrap justify-between gap-3 border-l-2 border-primary pl-4"
            >
              <StatusChip status={event.status as never} />
              <time className="font-mono text-caption text-text-muted">
                {new Date(event.changedAt).toLocaleString('en-GB')}
              </time>
            </li>
          ))}
        </ol>
      </Section>
    </>
  );
}
