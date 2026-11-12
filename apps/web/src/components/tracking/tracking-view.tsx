'use client';

import { Lock } from 'lucide-react';
import Link from 'next/link';
import { useRouter, useSearchParams } from 'next/navigation';
import { Suspense, useEffect, useRef, useState, type FormEvent } from 'react';

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
import {
  useLiveUpdates,
  type LiveState,
  type StatusUpdate,
} from '../../lib/use-live-updates';
import {
  serverFieldMessage,
  TRACKING_NUMBER_MESSAGE,
} from '../../lib/validation';
import { ApiProblemAlerts } from '../auth/api-problem-alerts';
import { RateLimitedAlert } from '../auth/form-alerts';
import { PageHeader } from '../layout/page-header';
import { Section } from '../layout/section';
import { Button, ButtonLink } from '../ui/button';
import { Card, CardContent } from '../ui/card';
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

const rules: FormRules<{ trackingNumber: string }> = {
  trackingNumber: ({ trackingNumber }) => {
    const number = normalizeTrackingNumber(trackingNumber);
    if (!number) return 'Enter a tracking number.';
    return isTrackingNumber(number) ? undefined : TRACKING_NUMBER_MESSAGE;
  },
};

const stepTitles: Record<ParcelStatus, string> = {
  CREATED: 'Created',
  PICKED_UP: 'Picked up',
  IN_TRANSIT: 'In transit',
  OUT_FOR_DELIVERY: 'Out for delivery',
  DELIVERED: 'Delivered',
  FAILED: 'Failed',
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

export function TrackingView() {
  return (
    <>
      {/* TODO(design-foundation): needs Breadcrumb (Home > Tracking, board page 21) */}
      <PageHeader
        title="Track a parcel"
        description="Enter a tracking number to see where a parcel is."
      />
      {/* The search reads the address, which a static page only knows in the browser. */}
      <Suspense>
        <TrackSearch />
      </Suspense>
    </>
  );
}

/**
 * A live update moves the parcel on: its status, its last scan and one more
 * history step. An older update or a repeat changes nothing. A newer update with
 * the same status only moves the last scan, so the history never shows the same
 * step twice. Only these three public fields are copied, so a socket message can
 * never add private text.
 */
function withUpdate(tracking: Tracking, update: StatusUpdate): Tracking {
  const isRepeat = tracking.timeline.some(
    (event) => event.status === update.status && event.at === update.updatedAt,
  );
  const isOlder =
    new Date(update.updatedAt).getTime() <
    new Date(tracking.lastUpdatedAt).getTime();
  if (isRepeat || isOlder) return tracking;
  if (update.status === tracking.status) {
    return { ...tracking, lastUpdatedAt: update.updatedAt };
  }
  return {
    ...tracking,
    status: update.status,
    lastUpdatedAt: update.updatedAt,
    timeline: [
      ...tracking.timeline,
      { status: update.status, at: update.updatedAt },
    ],
  };
}

/**
 * A loaded snapshot can arrive after a newer live update. It is the server's
 * answer, but an older one must not overwrite what the socket has just shown.
 */
function isOlderSnapshot(snapshot: Tracking, current: Tracking): boolean {
  return (
    new Date(snapshot.lastUpdatedAt).getTime() <
    new Date(current.lastUpdatedAt).getTime()
  );
}

/** The live connection in words, as M10's hook asks. */
const liveCaptions: Record<LiveState, string> = {
  live: 'Live. Changes appear here as they happen.',
  connecting: 'Connecting to live updates. Use Refresh to check now.',
  reconnecting: 'Connecting to live updates. Use Refresh to check now.',
  denied: 'Live updates are off. Use Refresh to check for changes.',
  limited: 'Live updates are off. Use Refresh to check for changes.',
};

/**
 * The number lives in the address (/track?number=SDK7M2Q9XP4), so a link can be
 * shared and the page refreshed. Everything shown comes from getTracking, which
 * keeps only cities, statuses and times (rule 11).
 */
function TrackSearch() {
  const router = useRouter();
  const number = normalizeTrackingNumber(useSearchParams().get('number') ?? '');
  const form = useForm({ trackingNumber: number }, rules);
  const { setValue, setServerErrors } = form;
  const [view, setView] = useState<View>({ status: 'idle' });
  // The address does not change when the same number is looked up again.
  const [attempt, setAttempt] = useState(0);
  // The last request wins: a quiet reload must not overwrite an answer from a
  // newer Refresh (or a newer lookup), even when both carry the same timestamp.
  const loadSeq = useRef(0);

  const problem = view.status === 'problem' ? view.problem : null;
  const remaining = useCountdown(
    problem?.kind === 'rate-limited' ? problem.seconds : null,
  );
  const isWaiting = problem?.kind === 'rate-limited' && remaining > 0;
  const isSearching = view.status === 'loading';
  const isRefreshing = view.status === 'found' && view.isRefreshing;
  const followed =
    view.status === 'found' ? view.tracking.trackingNumber : undefined;

  // Every load goes through getTracking, so only public fields are kept.
  const { state: liveState } = useLiveUpdates({
    trackingNumber: followed,
    onUpdate: (update) =>
      setView((current) =>
        current.status === 'found' &&
        current.tracking.trackingNumber === update.trackingNumber
          ? { ...current, tracking: withUpdate(current.tracking, update) }
          : current,
      ),
    // After each (re)connection the socket may have missed changes, so load the
    // parcel again quietly: no spinner, and a failure leaves the page as it is.
    onSync: () => {
      if (followed === undefined) return undefined;
      const requestId = (loadSeq.current += 1);
      return getTracking(followed)
        .then((tracking) => {
          if (requestId !== loadSeq.current) return;
          setView((current) => {
            if (
              current.status !== 'found' ||
              current.tracking.trackingNumber !== tracking.trackingNumber
            ) {
              return current;
            }
            return {
              ...current,
              tracking: isOlderSnapshot(tracking, current.tracking)
                ? current.tracking
                : tracking,
              isRefreshing: false,
            };
          });
        })
        .catch(() => {
          if (requestId !== loadSeq.current) return;
          setView((current) =>
            current.status === 'found'
              ? { ...current, isRefreshing: false }
              : current,
          );
        });
    },
  });

  // Only a new address replaces the field: Refresh keeps what is being typed.
  useEffect(() => {
    setValue('trackingNumber', number);
  }, [number, setValue]);

  useEffect(() => {
    if (!isTrackingNumber(number)) {
      setView({ status: 'idle' });
      if (number) setServerErrors({ trackingNumber: TRACKING_NUMBER_MESSAGE });
      return;
    }

    // Looking up the same parcel again keeps it on screen, and keeps focus on
    // the Refresh button, until the new answer arrives.
    setView((current) =>
      current.status === 'found' && current.tracking.trackingNumber === number
        ? { ...current, isRefreshing: true }
        : { status: 'loading' },
    );
    const requestId = (loadSeq.current += 1);
    const controller = new AbortController();
    getTracking(number, controller.signal)
      .then((tracking) => {
        if (requestId !== loadSeq.current) return;
        setView((current) =>
          // A manual Refresh also loads the parcel. If the socket already moved
          // it on while the request was in flight, keep the newer state.
          current.status === 'found' &&
          current.tracking.trackingNumber === tracking.trackingNumber &&
          isOlderSnapshot(tracking, current.tracking)
            ? { ...current, isRefreshing: false }
            : { status: 'found', tracking, isRefreshing: false },
        );
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted || requestId !== loadSeq.current) return;
        if (
          error instanceof ApiError &&
          (error.code === 'TRACKING_NOT_FOUND' || error.code === 'NOT_FOUND')
        ) {
          setView({ status: 'not-found' });
          return;
        }
        const classified = classifyApiError(error);
        const fieldMessage =
          classified.kind === 'validation' &&
          classified.issues.some((issue) => issue.field === 'trackingNumber')
            ? serverFieldMessage('trackingNumber')
            : undefined;
        if (fieldMessage) {
          setView({ status: 'idle' });
          setServerErrors({ trackingNumber: fieldMessage });
          return;
        }
        setView({ status: 'problem', problem: classified });
      });
    return () => controller.abort();
  }, [number, attempt, setServerErrors]);

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isWaiting || isSearching || isRefreshing) return;
    if (form.validate().trackingNumber) {
      document.getElementById(FIELD_ID)?.focus();
      return;
    }
    const next = normalizeTrackingNumber(form.values.trackingNumber);
    setValue('trackingNumber', next);
    if (next === number) setAttempt((count) => count + 1);
    else router.push(`/track?number=${encodeURIComponent(next)}`);
  }

  return (
    <div className="flex flex-col gap-8">
      <form noValidate onSubmit={submit} className="flex flex-col gap-3">
        <TextField
          id={FIELD_ID}
          label={
            view.status === 'found' ? 'Track another parcel' : 'Tracking number'
          }
          placeholder="SDK7M2Q9XP4"
          isRequired
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
        <Button
          type="submit"
          size="lg"
          isFullWidth
          isLoading={isSearching}
          isDisabled={isWaiting || isRefreshing}
        >
          {isSearching ? 'Tracking' : 'Track'}
        </Button>
      </form>

      <div aria-live="polite" className="flex flex-col gap-6">
        {problem?.kind === 'rate-limited' ? (
          <RateLimitedAlert remaining={remaining} isPublic />
        ) : (
          <ApiProblemAlerts problem={problem} />
        )}

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
          <Section title="Search result">
            <EmptyState
              title="No parcel with that number"
              action={
                <ButtonLink href="/track" variant="secondary" size="lg">
                  Clear search
                </ButtonLink>
              }
            >
              We could not find a parcel with that number. Check it and try
              again.
            </EmptyState>
          </Section>
        )}

        {view.status === 'found' && (
          <Found
            tracking={view.tracking}
            isRefreshing={view.isRefreshing}
            liveState={liveState}
            onRefresh={() => setAttempt((count) => count + 1)}
          />
        )}
      </div>
    </div>
  );
}

function Found({
  tracking,
  isRefreshing,
  liveState,
  onRefresh,
}: {
  tracking: Tracking;
  isRefreshing: boolean;
  liveState: LiveState | 'idle';
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
    <>
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
      <div className="flex flex-wrap gap-3">
        <Button variant="secondary" size="lg" onClick={copyNumber}>
          Copy number
        </Button>
        <Button
          variant="secondary"
          size="lg"
          isLoading={isRefreshing}
          onClick={onRefresh}
        >
          {isRefreshing ? 'Refreshing' : 'Refresh'}
        </Button>
      </div>
      {liveState !== 'idle' && (
        // The results container is already a polite live region, so the
        // caption has no role of its own and is read out only once.
        <p className="text-body-sm text-text-muted">
          {liveCaptions[liveState]}
        </p>
      )}
      <Card>
        <CardContent>
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
        </CardContent>
      </Card>
      <Card>
        <CardContent className="flex gap-3 text-body-sm text-text-muted">
          <Lock
            aria-hidden="true"
            strokeWidth={1.5}
            className="size-4 shrink-0"
          />
          <p>
            Is this your parcel?{' '}
            <Link
              href="/login"
              className="text-interactive underline underline-offset-4 hover:text-interactive-hover"
            >
              Sign in
            </Link>{' '}
            to see the recipient, the fee and the proof of delivery.
          </p>
        </CardContent>
      </Card>
    </>
  );
}
