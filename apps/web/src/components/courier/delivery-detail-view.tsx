'use client';

import { ArrowLeft, ClipboardCheck } from 'lucide-react';
import { useEffect, useRef, useState, type FormEvent } from 'react';

import { ApiError } from '../../lib/api';
import {
  getCourierDelivery,
  updateCourierDeliveryStatus,
  type CourierDeliveryDetail,
  type CourierTargetStatus,
} from '../../lib/courier-api';
import { formatDateTime, formatKg } from '../../lib/format';
import { sizeNames } from '../../lib/pricing-api';
import { type FormRules, useForm } from '../../lib/use-form';
import { useSession } from '../../lib/session';
import { useCountdown } from '../../lib/use-countdown';
import type { ParcelStatus } from '../../lib/tokens';
import { RateLimitedAlert, ServerFaultAlert } from '../auth/form-alerts';
import { PageHeader } from '../layout/page-header';
import { RequireRole } from '../layout/require-role';
import { Section } from '../layout/section';
import { SystemPage } from '../layout/system-page';
import { Alert } from '../ui/alert';
import { Button, ButtonLink } from '../ui/button';
import { Card, CardContent } from '../ui/card';
import { EmptyState } from '../ui/empty-state';
import { TextField } from '../ui/field';
import { KeyValueList } from '../ui/key-value-list';
import { Select } from '../ui/select';
import { Skeleton, SkeletonGroup } from '../ui/skeleton';
import { StatusChip, statusLabel } from '../ui/status-chip';
import { Textarea } from '../ui/textarea';
import { Timeline, type TimelineItem } from '../ui/timeline';
import { ProofUploadForm } from './proof-upload-form';

type DetailState =
  | { status: 'loading' }
  | {
      status: 'ready';
      delivery: CourierDeliveryDetail;
      isRefreshing: boolean;
      refreshError?: ApiError;
    }
  | { status: 'error'; error: ApiError };

type Values = {
  status: string;
  recipientName: string;
  failureReason: string;
  note: string;
};

const transitions: Partial<Record<ParcelStatus, CourierTargetStatus[]>> = {
  CREATED: ['PICKED_UP', 'FAILED'],
  PICKED_UP: ['IN_TRANSIT', 'FAILED'],
  IN_TRANSIT: ['OUT_FOR_DELIVERY', 'FAILED'],
  OUT_FOR_DELIVERY: ['DELIVERED', 'FAILED'],
};

const rules: FormRules<Values> = {
  status: ({ status }) => (status ? undefined : 'Choose the next status.'),
  recipientName: ({ status, recipientName }) =>
    status !== 'DELIVERED'
      ? undefined
      : !recipientName.trim()
        ? 'Enter the recipient name for a delivered parcel.'
        : Array.from(recipientName.trim()).length > 150
          ? 'Recipient name must be 150 characters or fewer.'
          : undefined,
  failureReason: ({ status, failureReason }) => {
    if (status !== 'FAILED') return undefined;
    const length = Array.from(failureReason.trim()).length;
    return length < 5 || length > 255
      ? 'Failure reason must be between 5 and 255 characters.'
      : undefined;
  },
  note: ({ note }) =>
    Array.from(note).length > 500
      ? 'Note must be 500 characters or fewer.'
      : undefined,
};

export function CourierDeliveryDetailView({ parcelId }: { parcelId: string }) {
  const { session } = useSession();
  const userId = session.status === 'authenticated' ? session.user.id : '';
  return (
    <RequireRole roles={['COURIER']} audience="couriers">
      <CourierDeliveryDetailContent
        key={`${userId}:${parcelId}`}
        parcelId={parcelId}
      />
    </RequireRole>
  );
}

function CourierDeliveryDetailContent({ parcelId }: { parcelId: string }) {
  const { recheck } = useSession();
  const [attempt, setAttempt] = useState(0);
  const [saved, setSaved] = useState(0);
  const [proofSaved, setProofSaved] = useState(0);
  const [state, setState] = useState<DetailState>({ status: 'loading' });

  useEffect(() => {
    const controller = new AbortController();
    setState((previous) =>
      previous.status === 'ready'
        ? { ...previous, isRefreshing: true, refreshError: undefined }
        : { status: 'loading' },
    );
    getCourierDelivery(parcelId, controller.signal)
      .then((delivery) => {
        if (!controller.signal.aborted)
          setState({ status: 'ready', delivery, isRefreshing: false });
      })
      .catch((caught: unknown) => {
        if (controller.signal.aborted) return;
        if (caught instanceof ApiError && caught.code === 'ACCOUNT_DEACTIVATED')
          recheck();
        const error =
          caught instanceof ApiError
            ? caught
            : new ApiError({ code: 'INTERNAL', status: 0 });
        setState((previous) =>
          previous.status === 'ready' &&
          (error.isServerFault || error.code === 'RATE_LIMITED')
            ? { ...previous, isRefreshing: false, refreshError: error }
            : { status: 'error', error },
        );
      });
    return () => controller.abort();
  }, [attempt, parcelId, recheck]);

  if (state.status === 'loading') return <DetailSkeleton />;
  if (state.status === 'error')
    return (
      <DetailError
        error={state.error}
        onRetry={() => setAttempt((count) => count + 1)}
      />
    );

  const { delivery } = state;
  return (
    <>
      <PageHeader
        title={delivery.trackingNumber}
        description="Assigned delivery details and current status."
        actions={
          <ButtonLink href="/dashboard" size="lg" variant="secondary">
            <ArrowLeft size={20} strokeWidth={1.5} aria-hidden="true" />
            Back to Today
          </ButtonLink>
        }
      />
      {saved > 0 && (
        <Alert tone="success" title="Delivery status saved">
          The status update has been recorded.
        </Alert>
      )}
      {proofSaved > 0 && (
        <Alert tone="success" title="Delivery proof uploaded">
          The proof has been stored securely.
        </Alert>
      )}
      {state.isRefreshing && (
        <p role="status" className="text-body text-text-muted">
          Refreshing delivery details
        </p>
      )}
      {state.refreshError && (
        <DetailError
          error={state.refreshError}
          isRefresh
          onRetry={() => setAttempt((count) => count + 1)}
        />
      )}
      <Section title="Delivery status">
        <Card>
          <CardContent className="flex flex-col gap-4">
            <StatusChip status={delivery.status} />
            <KeyValueList
              layout="inline"
              items={[
                {
                  label: 'Tracking number',
                  value: delivery.trackingNumber,
                  isMono: true,
                },
                {
                  label: 'Assigned',
                  value: formatDateTime(delivery.assignedAt),
                },
              ]}
            />
          </CardContent>
        </Card>
      </Section>
      <Section
        title="Update status"
        description="Only the next valid delivery statuses are available."
      >
        <StatusUpdateForm
          key={saved}
          parcelId={delivery.id}
          currentStatus={delivery.status}
          isRefreshing={state.isRefreshing || Boolean(state.refreshError)}
          onRefresh={() => setAttempt((count) => count + 1)}
          onUpdated={() => {
            setSaved((count) => count + 1);
            setAttempt((count) => count + 1);
          }}
        />
      </Section>
      <div className="grid min-w-0 gap-8 lg:grid-cols-2">
        <Section title="Sender and recipient">
          <KeyValueList
            layout="inline"
            items={[
              { label: 'Sender', value: delivery.senderName },
              { label: 'Sender phone', value: delivery.senderPhone },
              { label: 'Recipient', value: delivery.recipientName },
              { label: 'Recipient phone', value: delivery.recipientPhone },
            ]}
          />
        </Section>
        <Section title="Parcel details">
          <KeyValueList
            layout="inline"
            items={[
              { label: 'Pickup address', value: delivery.pickupAddress },
              {
                label: 'Drop-off address',
                value: [delivery.dropoffAddress, delivery.dropoffCity]
                  .filter((value): value is string => Boolean(value?.trim()))
                  .join(', '),
              },
              {
                label: 'Weight',
                value: formatKg(delivery.weightKg),
                isMono: true,
              },
              { label: 'Size', value: sizeNames[delivery.sizeCategory] },
              { label: 'Description', value: delivery.description },
            ]}
          />
        </Section>
      </div>
      <Section title="Status history">
        {delivery.history.length === 0 ? (
          <EmptyState title="No status history yet">
            Updates will appear here after the parcel changes status.
          </EmptyState>
        ) : (
          <Timeline
            label="Status history"
            className="break-words"
            items={timelineOf(delivery)}
          />
        )}
      </Section>
      <Section title="Delivery proof">
        <div className="flex flex-col gap-6">
          {delivery.proof ? (
            <KeyValueList
              layout="inline"
              items={[
                { label: 'Recipient', value: delivery.proof.recipientName },
                { label: 'Note', value: delivery.proof.note },
                {
                  label: 'Photo attached',
                  value: delivery.proof.hasPhoto ? 'Yes' : 'No',
                },
                {
                  label: 'Recorded',
                  value: formatDateTime(delivery.proof.createdAt),
                },
              ]}
            />
          ) : (
            <EmptyState title="No delivery proof recorded">
              Add a photo, recipient name or delivery note below.
            </EmptyState>
          )}
          <ProofUploadForm
            key={proofSaved}
            parcelId={delivery.id}
            onUploaded={() => {
              setProofSaved((count) => count + 1);
              setAttempt((count) => count + 1);
            }}
          />
        </div>
      </Section>
    </>
  );
}

function DetailSkeleton() {
  return (
    <SkeletonGroup
      label="Loading delivery details"
      className="flex flex-col gap-8"
    >
      <Skeleton className="h-10 w-2/3" />
      <div className="flex flex-col gap-4 rounded-card border border-line bg-surface p-6">
        <Skeleton className="h-6 w-1/3" />
        <Skeleton className="h-4 w-full" />
        <Skeleton className="h-4 w-2/3" />
      </div>
    </SkeletonGroup>
  );
}

function DetailError({
  error,
  onRetry,
  isRefresh = false,
}: {
  error: ApiError;
  onRetry: () => void;
  isRefresh?: boolean;
}) {
  const remaining = useCountdown(
    error.code === 'RATE_LIMITED' ? (error.retryAfter ?? null) : null,
  );
  if (error.code === 'FORBIDDEN') {
    return (
      <SystemPage
        label="403 Forbidden"
        title="You do not have access to this delivery"
        actions={
          <ButtonLink href="/dashboard" size="lg" variant="secondary">
            Back to Today
          </ButtonLink>
        }
      >
        This delivery is not assigned to you.
      </SystemPage>
    );
  }
  if (error.code === 'NOT_FOUND') {
    return (
      <SystemPage
        label="404 Not found"
        title="Delivery not found"
        actions={
          <ButtonLink href="/dashboard" size="lg" variant="secondary">
            Back to Today
          </ButtonLink>
        }
      >
        Check the delivery link and try again.
      </SystemPage>
    );
  }
  if (error.code === 'UNAUTHENTICATED' || error.code === 'ACCOUNT_DEACTIVATED')
    return null;
  return (
    <>
      {!isRefresh && <PageHeader title="Delivery details" />}
      <Section
        title={isRefresh ? 'Delivery refresh failed' : 'Delivery unavailable'}
      >
        {error.code === 'RATE_LIMITED' ? (
          <RateLimitedAlert remaining={remaining} />
        ) : error.isServerFault ? (
          <ServerFaultAlert
            isNetwork={error.code === 'NETWORK'}
            requestId={error.requestId}
          />
        ) : (
          <Alert tone="danger" title="Delivery details could not be loaded">
            Please try again.
          </Alert>
        )}
        <Button
          size="lg"
          variant="secondary"
          isDisabled={remaining > 0}
          onClick={onRetry}
        >
          Try again
        </Button>
      </Section>
    </>
  );
}

function timelineOf(delivery: CourierDeliveryDetail): TimelineItem[] {
  return delivery.history.map((entry, index) => ({
    id: `${entry.changedAt}-${entry.status}`,
    title: statusLabel(entry.status),
    at: entry.changedAt,
    detail:
      [entry.failureReason, entry.note].filter(Boolean).join(' — ') ||
      undefined,
    isCurrent: index === delivery.history.length - 1,
  }));
}

function StatusUpdateForm({
  parcelId,
  currentStatus,
  isRefreshing,
  onRefresh,
  onUpdated,
}: {
  parcelId: string;
  currentStatus: ParcelStatus;
  isRefreshing: boolean;
  onRefresh: () => void;
  onUpdated: () => void;
}) {
  const { recheck } = useSession();
  const request = useRef<AbortController | null>(null);
  const form = useForm<Values>(
    { status: '', recipientName: '', failureReason: '', note: '' },
    rules,
  );
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<ApiError | null>(null);
  const choices = transitions[currentStatus] ?? [];
  const remaining = useCountdown(
    error?.code === 'RATE_LIMITED' ? (error.retryAfter ?? null) : null,
  );
  const blocked =
    saving ||
    isRefreshing ||
    remaining > 0 ||
    error?.code === 'FORBIDDEN' ||
    error?.code === 'NOT_FOUND' ||
    error?.code === 'CONFLICT';
  useEffect(() => () => request.current?.abort(), []);

  if (choices.length === 0) {
    return (
      <Alert tone="info" title="This delivery has reached a final status">
        No further status update is available.
      </Alert>
    );
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (blocked || request.current) return;
    const errors = form.validate();
    const first = Object.keys(errors)[0] as keyof Values | undefined;
    if (first) {
      document.getElementById(`courier-status-${first}`)?.focus();
      return;
    }
    if (!choices.includes(form.values.status as CourierTargetStatus)) {
      form.setServerErrors({
        status: 'Choose a valid next status for this delivery.',
      });
      document.getElementById('courier-status-status')?.focus();
      return;
    }
    const controller = new AbortController();
    request.current = controller;
    setSaving(true);
    setError(null);
    try {
      await updateCourierDeliveryStatus(
        parcelId,
        {
          status: form.values.status as CourierTargetStatus,
          recipientName:
            form.values.status === 'DELIVERED'
              ? form.values.recipientName.trim()
              : undefined,
          failureReason:
            form.values.status === 'FAILED'
              ? form.values.failureReason.trim()
              : undefined,
          note: form.values.note || undefined,
        },
        controller.signal,
      );
      if (controller.signal.aborted) return;
      onUpdated();
    } catch (caught) {
      if (controller.signal.aborted) return;
      const next =
        caught instanceof ApiError
          ? caught
          : new ApiError({ code: 'INTERNAL', status: 0 });
      if (next.code === 'ACCOUNT_DEACTIVATED') recheck();
      setError(next);
    } finally {
      request.current = null;
      if (!controller.signal.aborted) setSaving(false);
    }
  }

  const isDelivered = form.values.status === 'DELIVERED';
  const isFailed = form.values.status === 'FAILED';
  return (
    <form
      noValidate
      onSubmit={submit}
      className="flex max-w-xl flex-col gap-4 [&_input]:min-h-12 [&_select]:min-h-12 [&_input:focus-visible]:outline-solid"
    >
      {error?.code === 'CONFLICT' ? (
        <Alert
          tone="warning"
          title="This delivery changed before you saved it"
          actions={
            <Button
              size="lg"
              variant="secondary"
              isLoading={isRefreshing}
              onClick={() => {
                setError(null);
                onRefresh();
              }}
            >
              Refresh delivery
            </Button>
          }
        >
          Your input is still here. Refresh the delivery and review its current
          status before trying again.
        </Alert>
      ) : error?.code === 'RATE_LIMITED' ? (
        <RateLimitedAlert remaining={remaining} />
      ) : (
        error && <StatusError error={error} />
      )}
      <Select
        id="courier-status-status"
        label="Next status"
        controlSize="lg"
        isRequired
        placeholder="Choose a status"
        options={choices.map((status) => ({
          value: status,
          label: statusLabel(status),
        }))}
        value={
          choices.includes(form.values.status as CourierTargetStatus)
            ? form.values.status
            : ''
        }
        error={form.errors.status}
        disabled={blocked}
        onChange={(event) => form.setValue('status', event.target.value)}
        onBlur={() => form.blur('status')}
      />
      {isDelivered && (
        <TextField
          id="courier-status-recipientName"
          label="Recipient name"
          controlSize="lg"
          isRequired
          name="recipientName"
          helper="Up to 150 characters."
          value={form.values.recipientName}
          error={form.errors.recipientName}
          disabled={blocked}
          onChange={(event) =>
            form.setValue('recipientName', event.target.value)
          }
          onBlur={() => form.blur('recipientName')}
        />
      )}
      {isFailed && (
        <Textarea
          id="courier-status-failureReason"
          label="Failure reason"
          controlSize="lg"
          isRequired
          name="failureReason"
          helper={`${Array.from(form.values.failureReason.trim()).length}/255 characters; at least 5 required.`}
          value={form.values.failureReason}
          error={form.errors.failureReason}
          disabled={blocked}
          onChange={(event) =>
            form.setValue('failureReason', event.target.value)
          }
          onBlur={() => form.blur('failureReason')}
        />
      )}
      <Textarea
        id="courier-status-note"
        label="Note"
        controlSize="lg"
        name="note"
        helper={`${Array.from(form.values.note).length}/500 characters. Optional.`}
        value={form.values.note}
        error={form.errors.note}
        disabled={blocked}
        onChange={(event) => form.setValue('note', event.target.value)}
        onBlur={() => form.blur('note')}
      />
      <Button
        type="submit"
        size="lg"
        isLoading={saving}
        isDisabled={!saving && blocked}
        iconLeading={
          <ClipboardCheck size={20} strokeWidth={1.5} aria-hidden="true" />
        }
      >
        {saving ? 'Saving status' : 'Save status'}
      </Button>
    </form>
  );
}

function StatusError({ error }: { error: ApiError }) {
  if (error.code === 'FORBIDDEN')
    return (
      <Alert tone="danger" title="You no longer have access to this delivery">
        Return to Today and contact your dispatcher if the assignment needs
        attention.
      </Alert>
    );
  if (error.code === 'VALIDATION_FAILED')
    return (
      <Alert tone="danger" title="Status update needs review">
        Review the next status, recipient name, failure reason and note, then
        try again. Your input is still here.
      </Alert>
    );
  if (error.code === 'NOT_FOUND')
    return (
      <Alert tone="danger" title="Delivery not found">
        Return to Today and select an assigned delivery.
      </Alert>
    );
  if (error.code === 'UNAUTHENTICATED' || error.code === 'ACCOUNT_DEACTIVATED')
    return null;
  if (error.isServerFault)
    return (
      <ServerFaultAlert
        isNetwork={error.code === 'NETWORK'}
        requestId={error.requestId}
      />
    );
  return (
    <Alert tone="danger" title="Status could not be saved">
      Review the delivery and try again.
    </Alert>
  );
}
