'use client';

import { ArrowLeft } from 'lucide-react';
import { useEffect, useState } from 'react';

import { ApiError } from '../../lib/api';
import {
  getCourierDelivery,
  type CourierDeliveryDetail,
} from '../../lib/courier-api';
import { formatDateTime, formatKg } from '../../lib/format';
import { sizeNames } from '../../lib/pricing-api';
import { useCountdown } from '../../lib/use-countdown';
import { useSession } from '../../lib/session';
import type { ParcelStatus } from '../../lib/tokens';
import { RateLimitedAlert, ServerFaultAlert } from '../auth/form-alerts';
import { PageHeader } from '../layout/page-header';
import { RequireRole } from '../layout/require-role';
import { Section } from '../layout/section';
import { SystemPage } from '../layout/system-page';
import { Alert } from '../ui/alert';
import { Button, ButtonLink } from '../ui/button';
import { Card, CardContent } from '../ui/card';
import { KeyValueList } from '../ui/key-value-list';
import { Skeleton, SkeletonGroup } from '../ui/skeleton';
import { StatusChip } from '../ui/status-chip';
import { Timeline, type TimelineItem } from '../ui/timeline';
import { DeliveryActivity } from './delivery-activity';
import {
  DeliveryActionForStatus,
  FailureAction,
  TerminalState,
} from './delivery-actions';
import { deliveryProgress, isActionable } from './delivery-progress';

type DetailState =
  | { status: 'loading' }
  | {
      status: 'ready';
      delivery: CourierDeliveryDetail;
      isRefreshing: boolean;
      refreshError?: ApiError;
    }
  | { status: 'error'; error: ApiError };

const actionTitles: Record<ParcelStatus, string> = {
  CREATED: 'Confirm pickup',
  PICKED_UP: 'Continue the delivery',
  IN_TRANSIT: 'Mark out for delivery',
  OUT_FOR_DELIVERY: 'Confirm delivery',
  DELIVERED: 'Delivered',
  FAILED: 'Delivery failed',
};

const actionDescriptions: Partial<Record<ParcelStatus, string>> = {
  CREATED: 'Add a pickup photo to start the delivery.',
  PICKED_UP: 'Pickup is already recorded. Continue without another photo.',
  IN_TRANSIT: 'Tell dispatch you are on the final leg.',
  OUT_FOR_DELIVERY: 'Record the handover with a recipient name and photo.',
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
  const [notice, setNotice] = useState<string | null>(null);
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
  const refresh = () => setAttempt((count) => count + 1);
  const isRefreshing = state.isRefreshing || Boolean(state.refreshError);
  const actionable = isActionable(delivery.status);

  return (
    <>
      <PageHeader
        title={delivery.trackingNumber}
        description="Assigned delivery details and progress."
        actions={
          <ButtonLink href="/dashboard" size="lg" variant="secondary">
            <ArrowLeft size={20} strokeWidth={1.5} aria-hidden="true" />
            Back to deliveries
          </ButtonLink>
        }
      />

      <Card>
        <CardContent className="flex flex-wrap items-center gap-x-6 gap-y-3">
          <StatusChip status={delivery.status} />
          <p className="text-body-sm text-text-muted">
            Assigned {formatDateTime(delivery.assignedAt)}
          </p>
        </CardContent>
      </Card>

      {state.isRefreshing && (
        <p role="status" className="text-body text-text-muted">
          Refreshing delivery details
        </p>
      )}
      {state.refreshError && (
        <DetailError error={state.refreshError} isRefresh onRetry={refresh} />
      )}

      <Section title="Delivery progress">
        <Timeline label="Delivery progress" items={progressItems(delivery)} />
      </Section>

      <div className="grid min-w-0 gap-8 lg:grid-cols-2">
        <Section title="Sender and recipient">
          <KeyValueList
            items={[
              { label: 'Sender', value: delivery.senderName },
              { label: 'Sender phone', value: delivery.senderPhone },
              { label: 'Pickup address', value: delivery.pickupAddress },
              { label: 'Recipient', value: delivery.recipientName },
              { label: 'Recipient phone', value: delivery.recipientPhone },
              {
                label: 'Drop-off address',
                value: [delivery.dropoffAddress, delivery.dropoffCity]
                  .filter((value): value is string => Boolean(value?.trim()))
                  .join(', '),
              },
            ]}
          />
        </Section>
        <Section title="Parcel details">
          <KeyValueList
            items={[
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

      <Section
        title={actionTitles[delivery.status]}
        description={actionDescriptions[delivery.status]}
      >
        {notice && (
          <Alert tone="success" title="Delivery updated">
            {notice}
          </Alert>
        )}
        {actionable ? (
          <div className="flex flex-col gap-4">
            <DeliveryActionForStatus
              delivery={delivery}
              isRefreshing={isRefreshing}
              onConfirmed={(message) => {
                setNotice(message);
                refresh();
              }}
              onRefresh={refresh}
            />
            <div>
              <FailureAction
                parcelId={delivery.id}
                isRefreshing={isRefreshing}
                onConfirmed={(message) => {
                  setNotice(message);
                  refresh();
                }}
                onRefresh={refresh}
              />
            </div>
          </div>
        ) : (
          <TerminalState delivery={delivery} />
        )}
      </Section>

      <DeliveryActivity delivery={delivery} />
    </>
  );
}

function progressItems(delivery: CourierDeliveryDetail): TimelineItem[] {
  return deliveryProgress(delivery.status, delivery.history).map((step) => ({
    id: step.id,
    title: step.title,
    at: step.at,
    detail:
      step.state === 'failed' && step.detail ? (
        <span className="text-danger">{step.detail}</span>
      ) : (
        step.detail
      ),
    isCurrent: step.state === 'current' || step.state === 'failed',
    isUpcoming: step.state === 'upcoming',
  }));
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
            Back to deliveries
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
            Back to deliveries
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
