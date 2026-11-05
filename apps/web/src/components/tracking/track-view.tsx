'use client';

import { Copy, Lock, RefreshCw } from 'lucide-react';
import Link from 'next/link';
import { useRouter, useSearchParams } from 'next/navigation';
import { useEffect, useState, type FormEvent } from 'react';

import { ApiError } from '../../lib/api-errors';
import { classifyApiError, type ApiProblem } from '../../lib/api-problem';
import { formatDate, formatDateTime } from '../../lib/format';
import type { ParcelStatus } from '../../lib/tokens';
import {
  getTracking,
  isTrackingNumber,
  normalizeTrackingNumber,
  type Tracking,
} from '../../lib/tracking-api';
import { useCountdown } from '../../lib/use-countdown';
import { useForm, type FormRules } from '../../lib/use-form';
import { ApiProblemAlerts } from '../auth/api-problem-alerts';
import { PageHeader } from '../layout/page-header';
import { Section } from '../layout/section';
import { Button, ButtonLink } from '../ui/button';
import { EmptyState } from '../ui/empty-state';
import { TextField } from '../ui/field';
import { Timeline, type TimelineItem } from '../ui/timeline';
import { useToast } from '../ui/toast';
import { WaybillStub } from '../ui/waybill-stub';

type View =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'found'; tracking: Tracking; isRefreshing: boolean }
  | { status: 'not-found' }
  | { status: 'problem'; problem: ApiProblem };

const FIELD_ID = 'tracking-number';
const FORMAT_MESSAGE =
  'Check the number. Ours are 11 characters, starting SDK or SDN.';

const rules: FormRules<{ trackingNumber: string }> = {
  trackingNumber: ({ trackingNumber }) => {
    const number = normalizeTrackingNumber(trackingNumber);
    if (!number) return 'Enter a tracking number.';
    return isTrackingNumber(number) ? undefined : FORMAT_MESSAGE;
  },
};

const stepTitles: Record<ParcelStatus, string> = {
  CREATED: 'Parcel created',
  PICKED_UP: 'Collected from the sender',
  IN_TRANSIT: 'In transit',
  OUT_FOR_DELIVERY: 'Out for delivery',
  DELIVERED: 'Delivered',
  FAILED: 'Delivery failed',
};

const route: ParcelStatus[] = [
  'CREATED',
  'PICKED_UP',
  'IN_TRANSIT',
  'OUT_FOR_DELIVERY',
  'DELIVERED',
];

/** What has happened, then the steps still to come. A failed parcel has none. */
function historyOf({ status, timeline }: Tracking): TimelineItem[] {
  const done = timeline.map((event, index): TimelineItem => ({
    id: `${event.status}-${event.at}`,
    title: stepTitles[event.status],
    at: event.at,
    isCurrent: index === timeline.length - 1,
  }));
  const upcoming =
    status === 'FAILED' ? [] : route.slice(route.indexOf(status) + 1);
  return [
    ...done,
    ...upcoming.map((step): TimelineItem => ({
      id: step,
      title: stepTitles[step],
      detail: step === 'DELIVERED' ? 'Final step' : 'Not yet reached',
      isUpcoming: true,
    })),
  ];
}

/**
 * The number lives in the address (/track?number=SDK7M2Q9XP4), so a link can be
 * shared and the page refreshed. Everything shown comes from getTracking, which
 * keeps only cities, statuses and times (rule 11).
 */
export function TrackView() {
  const router = useRouter();
  const number = normalizeTrackingNumber(useSearchParams().get('number') ?? '');
  const form = useForm({ trackingNumber: number }, rules);
  const { setValue, setServerErrors } = form;
  const [view, setView] = useState<View>({ status: 'idle' });
  // The address does not change when the same number is looked up again.
  const [attempt, setAttempt] = useState(0);

  const problem = view.status === 'problem' ? view.problem : null;
  const remaining = useCountdown(
    problem?.kind === 'rate-limited' ? problem.seconds : null,
  );
  const isWaiting = problem?.kind === 'rate-limited' && remaining > 0;
  const isBusy =
    view.status === 'loading' || (view.status === 'found' && view.isRefreshing);

  useEffect(() => {
    setValue('trackingNumber', number);
    if (!isTrackingNumber(number)) {
      setView({ status: 'idle' });
      if (number) setServerErrors({ trackingNumber: FORMAT_MESSAGE });
      return;
    }

    // Looking up the same parcel again keeps it on screen, and keeps focus on
    // the Refresh button, until the new answer arrives.
    setView((current) =>
      current.status === 'found' && current.tracking.trackingNumber === number
        ? { ...current, isRefreshing: true }
        : { status: 'loading' },
    );
    const controller = new AbortController();
    getTracking(number, controller.signal)
      .then((tracking) =>
        setView({ status: 'found', tracking, isRefreshing: false }),
      )
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        if (
          error instanceof ApiError &&
          (error.code === 'TRACKING_NOT_FOUND' || error.code === 'NOT_FOUND')
        ) {
          setView({ status: 'not-found' });
          return;
        }
        const classified = classifyApiError(error);
        if (classified.kind === 'validation') {
          setView({ status: 'idle' });
          setServerErrors({ trackingNumber: FORMAT_MESSAGE });
          return;
        }
        setView({ status: 'problem', problem: classified });
      });
    return () => controller.abort();
  }, [number, attempt, setValue, setServerErrors]);

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isWaiting || isBusy) return;
    if (form.validate().trackingNumber) {
      document.getElementById(FIELD_ID)?.focus();
      return;
    }
    const next = normalizeTrackingNumber(form.values.trackingNumber);
    setValue('trackingNumber', next);
    if (next === number) setAttempt((count) => count + 1);
    else router.push(`/track?number=${encodeURIComponent(next)}`);
  }

  const isFound = view.status === 'found';

  return (
    <>
      {/* TODO(design-foundation): needs Breadcrumb (Home > Tracking, board page 21) */}
      <div className="flex flex-wrap items-end justify-between gap-6">
        <PageHeader
          title={isFound ? `Parcel ${number}` : 'Track a parcel'}
          description={
            isFound
              ? undefined
              : 'Enter a tracking number to see where a parcel is.'
          }
        />
        <form
          noValidate
          onSubmit={submit}
          className="flex w-full flex-col gap-3 md:w-96 md:flex-row md:items-start"
        >
          <div className="min-w-0 flex-1">
            <TextField
              id={FIELD_ID}
              label={isFound ? 'Track another parcel' : 'Tracking number'}
              placeholder="SDK7M2Q9XP4"
              isMono
              controlSize="lg"
              autoComplete="off"
              autoCapitalize="characters"
              spellCheck={false}
              value={form.values.trackingNumber}
              error={form.errors.trackingNumber}
              onChange={(event) =>
                form.setValue('trackingNumber', event.target.value)
              }
              onBlur={() => form.blur('trackingNumber')}
            />
          </div>
          <Button
            type="submit"
            size="lg"
            className="md:mt-6"
            isLoading={isBusy}
            isDisabled={isWaiting}
          >
            {isBusy ? 'Tracking' : 'Track'}
          </Button>
        </form>
      </div>

      <div aria-live="polite" className="flex flex-col gap-6">
        <ApiProblemAlerts problem={problem} remaining={remaining} />

        {view.status === 'loading' && (
          <WaybillStub
            isLoading
            isPublic
            trackingNumber={number}
            status="CREATED"
            origin=""
            destination=""
          />
        )}

        {view.status === 'not-found' && (
          <EmptyState
            title="No parcel with that number"
            action={
              <ButtonLink href="/track" variant="secondary">
                Clear search
              </ButtonLink>
            }
          >
            {FORMAT_MESSAGE}
          </EmptyState>
        )}

        {view.status === 'found' && (
          <Found
            tracking={view.tracking}
            isRefreshing={view.isRefreshing}
            onRefresh={() => setAttempt((count) => count + 1)}
          />
        )}
      </div>
    </>
  );
}

function Found({
  tracking,
  isRefreshing,
  onRefresh,
}: {
  tracking: Tracking;
  isRefreshing: boolean;
  onRefresh: () => void;
}) {
  const { show } = useToast();
  const created = tracking.timeline[0]?.at;

  async function copyNumber() {
    try {
      await navigator.clipboard.writeText(tracking.trackingNumber);
      show({ title: 'Tracking number copied.' });
    } catch {
      show({
        title: 'We could not copy the number.',
        description: 'Select it on the page and copy it yourself.',
        tone: 'danger',
      });
    }
  }

  return (
    <div className="grid items-start gap-6 lg:grid-cols-2">
      <div className="flex min-w-0 flex-col gap-4">
        <WaybillStub
          isPublic
          trackingNumber={tracking.trackingNumber}
          status={tracking.status}
          origin={tracking.pickupCity}
          destination={tracking.dropoffCity}
          footRows={[
            {
              label: 'Created',
              value: created ? formatDate(created) : '',
              isMono: true,
            },
            {
              label: 'Last scan',
              value: formatDateTime(tracking.lastUpdatedAt),
              isMono: true,
            },
          ]}
        />
        <p className="flex gap-3 rounded-card border border-line bg-surface p-4 text-body-sm text-text-muted">
          <Lock
            aria-hidden="true"
            strokeWidth={1.5}
            className="size-4 shrink-0"
          />
          <span>
            Is this your parcel?{' '}
            <Link
              href="/login"
              className="text-interactive underline underline-offset-4 hover:text-interactive-hover"
            >
              Sign in
            </Link>{' '}
            to see the recipient, the fee and the proof of delivery.
          </span>
        </p>
        <div className="flex flex-wrap gap-3">
          <Button
            variant="secondary"
            iconLeading={
              <Copy aria-hidden="true" strokeWidth={1.5} className="size-4" />
            }
            onClick={copyNumber}
          >
            Copy number
          </Button>
          <Button
            variant="secondary"
            isLoading={isRefreshing}
            iconLeading={
              <RefreshCw
                aria-hidden="true"
                strokeWidth={1.5}
                className="size-4"
              />
            }
            onClick={onRefresh}
          >
            {isRefreshing ? 'Refreshing' : 'Refresh'}
          </Button>
        </div>
      </div>

      <div className="min-w-0 rounded-card border border-line bg-surface p-4 md:p-6">
        <Section
          title="Status history"
          actions={
            <span className="font-mono text-caption text-text-muted">
              times in UTC+5:30
            </span>
          }
        >
          <Timeline label="Status history" items={historyOf(tracking)} />
        </Section>
      </div>
    </div>
  );
}
