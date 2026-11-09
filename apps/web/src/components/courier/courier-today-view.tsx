'use client';

import { PackageOpen, RefreshCw } from 'lucide-react';
import { useEffect, useState } from 'react';

import { ApiError } from '../../lib/api';
import {
  listCourierDeliveries,
  type CourierDelivery,
  type CourierDeliveryPage,
} from '../../lib/courier-api';
import { useSession } from '../../lib/session';
import { useCountdown } from '../../lib/use-countdown';
import { RateLimitedAlert, ServerFaultAlert } from '../auth/form-alerts';
import { PageHeader } from '../layout/page-header';
import { RequireRole } from '../layout/require-role';
import { Section } from '../layout/section';
import { Alert } from '../ui/alert';
import { Button, ButtonLink } from '../ui/button';
import {
  Card,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
} from '../ui/card';
import { EmptyState } from '../ui/empty-state';
import { KeyValueList } from '../ui/key-value-list';
import {
  Pagination,
  PaginationContent,
  PaginationItem,
  PaginationNext,
  PaginationPrevious,
} from '../ui/pagination';
import { Skeleton, SkeletonGroup } from '../ui/skeleton';
import { StatusChip } from '../ui/status-chip';

type LoadState = { key: string } & (
  | { status: 'loading' }
  | { status: 'ready'; deliveries: CourierDeliveryPage }
  | { status: 'error'; error: ApiError }
);

function todayInColombo() {
  const parts = new Intl.DateTimeFormat('en-GB', {
    timeZone: 'Asia/Colombo',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).formatToParts(new Date());
  const part = (name: string) =>
    parts.find((item) => item.type === name)?.value;
  return `${part('year')}-${part('month')}-${part('day')}`;
}

export function CourierTodayView() {
  return (
    <RequireRole roles={['COURIER']} audience="couriers">
      <CourierTodayContent />
    </RequireRole>
  );
}

function CourierTodayContent() {
  const { session, recheck } = useSession();
  const userId = session.status === 'authenticated' ? session.user.id : '';
  const [attempt, setAttempt] = useState(0);
  const [page, setPage] = useState(1);
  const [state, setState] = useState<LoadState>({ key: '', status: 'loading' });
  const requestKey = JSON.stringify([userId, page, attempt]);

  useEffect(() => {
    if (!userId) return;
    const controller = new AbortController();
    setState({ key: requestKey, status: 'loading' });
    listCourierDeliveries({
      date: todayInColombo(),
      page,
      signal: controller.signal,
    })
      .then((deliveries) => {
        if (!controller.signal.aborted)
          setState({ key: requestKey, status: 'ready', deliveries });
      })
      .catch((caught: unknown) => {
        if (controller.signal.aborted) return;
        const error =
          caught instanceof ApiError
            ? caught
            : new ApiError({ code: 'INTERNAL', status: 0 });
        if (error.code === 'ACCOUNT_DEACTIVATED') recheck();
        setState({ key: requestKey, status: 'error', error });
      });
    return () => controller.abort();
  }, [page, recheck, userId, requestKey]);

  const current = state.key === requestKey ? state : null;
  const error = current?.status === 'error' ? current.error : null;
  const isLoading = !current || current.status === 'loading';
  const result = current?.status === 'ready' ? current.deliveries : null;
  const deliveries = result?.items ?? [];
  const noPermission = error?.code === 'FORBIDDEN';
  const remaining = useCountdown(
    error?.code === 'RATE_LIMITED' ? (error.retryAfter ?? null) : null,
  );

  return (
    <>
      <PageHeader
        title="Today"
        description="Deliveries currently assigned to you."
        actions={
          !noPermission && (
            <Button
              size="lg"
              variant="secondary"
              isLoading={isLoading}
              isDisabled={remaining > 0}
              onClick={() => setAttempt((count) => count + 1)}
              iconLeading={
                <RefreshCw size={20} strokeWidth={1.5} aria-hidden="true" />
              }
            >
              {isLoading ? 'Refreshing' : 'Refresh'}
            </Button>
          )
        }
      />
      <Section
        title="Assigned deliveries"
        description="Open a delivery to update its status."
      >
        {isLoading ? (
          <DeliverySkeleton />
        ) : error ? (
          <TodayError error={error} remaining={remaining} />
        ) : deliveries.length === 0 ? (
          <EmptyState title="No assigned deliveries" icon={PackageOpen}>
            You have no deliveries assigned right now. Check again after a
            dispatcher assigns one.
          </EmptyState>
        ) : (
          <ul className="grid min-w-0 gap-4 lg:grid-cols-2">
            {deliveries.map((delivery) => (
              <li key={delivery.id}>
                <DeliveryCard delivery={delivery} />
              </li>
            ))}
          </ul>
        )}
        {!error && (page > 1 || (result?.pagination.totalPages ?? 0) > 1) && (
          <Pagination
            aria-label="Assigned delivery pages"
            className="flex-wrap justify-between gap-4"
          >
            <p className="text-body text-text-muted">
              Page {page}
              {result && ` of ${result.pagination.totalPages}`}
            </p>
            <PaginationContent className="gap-2">
              <PaginationItem>
                <PaginationPrevious
                  size="lg"
                  className="min-w-12"
                  disabled={isLoading || page <= 1}
                  onClick={() => setPage((value) => value - 1)}
                />
              </PaginationItem>
              <PaginationItem>
                <PaginationNext
                  size="lg"
                  className="min-w-12"
                  disabled={
                    isLoading || !result || page >= result.pagination.totalPages
                  }
                  onClick={() => setPage((value) => value + 1)}
                />
              </PaginationItem>
            </PaginationContent>
          </Pagination>
        )}
      </Section>
    </>
  );
}

function DeliverySkeleton() {
  return (
    <SkeletonGroup
      label="Loading assigned deliveries"
      className="grid gap-4 lg:grid-cols-2"
    >
      {Array.from({ length: 3 }, (_, index) => (
        <div
          key={index}
          className="flex flex-col gap-4 rounded-card border border-line bg-surface p-6"
        >
          <Skeleton className="h-5 w-2/5" />
          <Skeleton className="h-6 w-1/3" />
          <Skeleton className="h-4 w-full" />
          <Skeleton className="h-12 w-full" />
        </div>
      ))}
    </SkeletonGroup>
  );
}

function DeliveryCard({ delivery }: { delivery: CourierDelivery }) {
  const destination = [delivery.dropoffAddress, delivery.dropoffCity]
    .filter((value): value is string => Boolean(value?.trim()))
    .join(', ');
  return (
    <Card className="gap-4">
      <CardHeader>
        <CardTitle className="break-all font-mono text-h4">
          {delivery.trackingNumber}
        </CardTitle>
        <StatusChip status={delivery.status} />
      </CardHeader>
      <CardContent>
        <KeyValueList
          items={[
            { label: 'Pickup', value: delivery.pickupAddress },
            { label: 'Drop-off', value: destination },
          ]}
        />
      </CardContent>
      <CardFooter>
        <ButtonLink
          href={`/deliveries/${delivery.id}`}
          size="lg"
          variant="secondary"
          isFullWidth
        >
          View delivery
        </ButtonLink>
      </CardFooter>
    </Card>
  );
}

function TodayError({
  error,
  remaining,
}: {
  error: ApiError;
  remaining: number;
}) {
  if (error.code === 'UNAUTHENTICATED' || error.code === 'ACCOUNT_DEACTIVATED')
    return null;
  if (error.code === 'FORBIDDEN') {
    return (
      <Alert
        tone="danger"
        title="You do not have access to assigned deliveries"
      >
        This view is limited to couriers. Contact your dispatcher if you need
        access.
      </Alert>
    );
  }
  if (error.isServerFault) {
    return (
      <ServerFaultAlert
        isNetwork={error.code === 'NETWORK'}
        requestId={error.requestId}
      />
    );
  }
  if (error.code === 'RATE_LIMITED')
    return <RateLimitedAlert remaining={remaining} />;
  return (
    <Alert tone="danger" title="Assigned deliveries could not be loaded">
      Please refresh and try again.
    </Alert>
  );
}
