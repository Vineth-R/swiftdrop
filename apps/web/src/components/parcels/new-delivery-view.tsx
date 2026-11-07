'use client';

import { useEffect, useRef, useState, type FormEvent } from 'react';
import { ApiError } from '../../lib/api';
import { createParcel, type CreatedParcel } from '../../lib/parcels-api';
import {
  estimate,
  getZones,
  sizeCategories,
  type Estimate,
  type SizeCategory,
  type Zone,
} from '../../lib/pricing-api';
import { useCountdown } from '../../lib/use-countdown';
import { useForm } from '../../lib/use-form';
import { serverFieldMessage } from '../../lib/validation';
import { formatKg, formatLkr } from '../../lib/format';
import { PageHeader } from '../layout/page-header';
import { RequireRole } from '../layout/require-role';
import { Section } from '../layout/section';
import { Alert } from '../ui/alert';
import { Button, ButtonLink } from '../ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '../ui/dialog';
import { EmptyState } from '../ui/empty-state';
import { TextField } from '../ui/field';
import { KeyValueList } from '../ui/key-value-list';
import { Select } from '../ui/select';
import { SkeletonGroup, SkeletonText } from '../ui/skeleton';
import { Textarea } from '../ui/textarea';
import { ContactFields } from './contact-fields';
import {
  deliveryRules,
  initialValues,
  parcelInput,
  routeFields,
  type DeliveryValues,
} from './delivery-form';
import { FeeBreakdown } from './fee-breakdown';
import {
  ParcelProblem,
  parcelMessages,
  rateLimitSeconds,
} from './parcel-problem';

/** The estimate's own fields, as the wizard names them. */
const estimateFields: Record<string, keyof DeliveryValues> = {
  originZoneId: 'senderZone',
  destinationZoneId: 'recipientZone',
  weightKg: 'weight',
  sizeCategory: 'size',
};

/**
 * Where an estimate problem belongs: beside the field it is about, when there
 * is one. An empty result means it stays an alert.
 */
function estimateFieldErrors(
  error: unknown,
): Partial<Record<keyof DeliveryValues, string>> {
  if (!(error instanceof ApiError)) return {};
  // The service reports the origin/destination pair, not which zone is wrong.
  // Destination is where the wizard asks for the route being priced, so that is
  // where the message goes; the contract gives no way to split the pair.
  if (error.code === 'ROUTE_NOT_SERVICEABLE')
    return { recipientZone: parcelMessages.ROUTE_NOT_SERVICEABLE };
  if (error.code === 'WEIGHT_NOT_PRICED')
    return { weight: parcelMessages.WEIGHT_NOT_PRICED };
  if (error.code !== 'VALIDATION_FAILED') return {};
  const errors: Partial<Record<keyof DeliveryValues, string>> = {};
  for (const issue of error.issues) {
    const field = estimateFields[issue.field];
    if (field)
      errors[field] =
        serverFieldMessage(issue.field) ??
        'Check this information and try again.';
  }
  return errors;
}

type QuoteState =
  | { status: 'idle' | 'loading' }
  | { status: 'ready'; fee: Estimate; fingerprint: string }
  | { status: 'error'; error: unknown };

export function NewDeliveryView() {
  return (
    <RequireRole roles={['CUSTOMER']} audience="customers">
      <DeliveryWizard />
    </RequireRole>
  );
}

function DeliveryWizard() {
  const form = useForm(initialValues, deliveryRules);
  const [step, setStep] = useState(0);
  const [zones, setZones] = useState<Zone[] | null>(null);
  const [zoneError, setZoneError] = useState<unknown>(null);
  const [reload, setReload] = useState(0);
  const [quote, setQuote] = useState<QuoteState>({ status: 'idle' });
  const [problem, setProblem] = useState<unknown>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [isUncertain, setIsUncertain] = useState(false);
  const [created, setCreated] = useState<CreatedParcel | null>(null);
  const [changedFee, setChangedFee] = useState<number | null>(null);
  const key = useRef<string | null>(null);
  const wasRejected = useRef(false);
  const inFlight = useRef(false);
  const heading = useRef<HTMLDivElement>(null);
  const values = form.values;
  // Estimate problems live apart from the form's client and create-form errors,
  // so one source can never clear the other.
  const [estimateErrors, setEstimateErrors] = useState<
    Partial<Record<keyof DeliveryValues, string>>
  >({});
  const estimateRetry =
    quote.status === 'error' ? rateLimitSeconds(quote.error) : null;
  const estimateWait = useCountdown(estimateRetry);
  const createWait = useCountdown(rateLimitSeconds(problem));
  // The countdown is display state; this ref stops a new estimate while it runs.
  const estimateBlocked = useRef(false);
  const fingerprint = JSON.stringify([
    values.senderZone,
    values.recipientZone,
    values.weight,
    values.size,
  ]);
  const isRouteValid = routeFields.every(
    (field) => !deliveryRules[field](values),
  );

  useEffect(() => {
    estimateBlocked.current = estimateWait > 0;
  }, [estimateWait]);

  useEffect(() => {
    const controller = new AbortController();
    getZones(controller.signal)
      .then(setZones)
      .catch((error: unknown) => {
        if (!controller.signal.aborted) setZoneError(error);
      });
    return () => controller.abort();
  }, [reload]);

  useEffect(() => {
    if (!isRouteValid || estimateBlocked.current) return;
    const controller = new AbortController();
    const timer = setTimeout(() => {
      setQuote({ status: 'loading' });
      estimate(
        {
          originZoneId: values.senderZone,
          destinationZoneId: values.recipientZone,
          weightKg: Number(values.weight),
          sizeCategory: values.size as SizeCategory,
        },
        controller.signal,
      )
        .then((fee) => {
          if (controller.signal.aborted) return;
          setEstimateErrors({});
          setQuote({ status: 'ready', fee, fingerprint });
        })
        .catch((error: unknown) => {
          if (controller.signal.aborted) return;
          setEstimateErrors(estimateFieldErrors(error));
          setQuote({ status: 'error', error });
        });
    }, 350);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
    // The fingerprint contains exactly the four pricing inputs.
  }, [
    fingerprint,
    isRouteValid,
    reload,
    values.senderZone,
    values.recipientZone,
    values.weight,
    values.size,
  ]);

  function go(next: number) {
    setStep(next);
    requestAnimationFrame(() => heading.current?.focus());
  }
  function edit(field: keyof DeliveryValues, value: string) {
    if (isUncertain) return;
    if (wasRejected.current && value !== values[field]) {
      // An explicit edit after a definite rejection starts a new intended
      // request. Retrying an unchanged request keeps its original key.
      key.current = null;
      wasRejected.current = false;
    }
    form.setValue(field, value);
    // A rate limit belongs to the endpoint, not to these values: editing a
    // field must not clear its countdown.
    if (rateLimitSeconds(problem) === null) setProblem(null);
  }
  function next(event: FormEvent) {
    event.preventDefault();
    const errors =
      step === 0
        ? Object.fromEntries(
            routeFields.map((field) => [field, deliveryRules[field](values)]),
          )
        : form.validate();
    if (step === 0) routeFields.forEach((field) => form.blur(field));
    const fields =
      step === 0
        ? routeFields
        : (Object.keys(deliveryRules) as (keyof DeliveryValues)[]);
    const invalid = fields.find((field) => errors[field]);
    if (invalid) {
      document.getElementById(`delivery-${invalid}`)?.focus();
      return;
    }
    if (quote.status !== 'ready' || quote.fingerprint !== fingerprint) return;
    go(step + 1);
  }

  async function submit() {
    if (
      inFlight.current ||
      createWait > 0 ||
      quote.status !== 'ready' ||
      quote.fingerprint !== fingerprint
    )
      return;
    inFlight.current = true;
    setIsLoading(true);
    setProblem(null);
    key.current ??= crypto.randomUUID();
    try {
      setCreated(
        await createParcel(parcelInput(values, quote.fee.total), key.current),
      );
    } catch (error) {
      const uncertain =
        !(error instanceof ApiError) ||
        error.code === 'NETWORK' ||
        error.status >= 500;
      setIsUncertain(uncertain);
      wasRejected.current = !uncertain;
      if (
        error instanceof ApiError &&
        error.code === 'PRICE_CHANGED' &&
        error.currentFee !== undefined
      )
        setChangedFee(error.currentFee);
      else {
        setProblem(error);
        if (error instanceof ApiError && error.code === 'VALIDATION_FAILED') {
          const errors: Partial<Record<keyof DeliveryValues, string>> = {};
          const mapping: Record<string, keyof DeliveryValues> = {
            'sender.name': 'senderName',
            'sender.phone': 'senderPhone',
            'sender.address': 'senderAddress',
            'sender.city': 'senderCity',
            'sender.zoneId': 'senderZone',
            'recipient.name': 'recipientName',
            'recipient.phone': 'recipientPhone',
            'recipient.address': 'recipientAddress',
            'recipient.city': 'recipientCity',
            'recipient.zoneId': 'recipientZone',
            weightKg: 'weight',
            sizeCategory: 'size',
            description: 'description',
          };
          for (const issue of error.issues) {
            const field = mapping[issue.field];
            if (field)
              errors[field] =
                serverFieldMessage(issue.field) ??
                'Check this information and try again.';
          }
          form.setServerErrors(errors);
          const first = Object.keys(errors)[0] as
            keyof DeliveryValues | undefined;
          if (first) {
            setStep(routeFields.includes(first) ? 0 : 1);
            requestAnimationFrame(() =>
              document.getElementById(`delivery-${first}`)?.focus(),
            );
          }
        }
      }
    } finally {
      inFlight.current = false;
      setIsLoading(false);
    }
  }

  async function confirmPrice() {
    if (changedFee === null) return;
    setChangedFee(null);
    setQuote({ status: 'loading' });
    try {
      const fee = await estimate({
        originZoneId: values.senderZone,
        destinationZoneId: values.recipientZone,
        weightKg: Number(values.weight),
        sizeCategory: values.size as SizeCategory,
      });
      setEstimateErrors({});
      setQuote({ status: 'ready', fee, fingerprint });
    } catch (error) {
      setEstimateErrors(estimateFieldErrors(error));
      setQuote({ status: 'error', error });
    }
  }

  if (created)
    return (
      <>
        <PageHeader
          title="Delivery created"
          description="Your delivery request has been saved."
        />
        <Section title="Your tracking number">
          <Alert tone="success" title="Ready for collection">
            <span className="break-all font-mono">
              {created.trackingNumber}
            </span>
          </Alert>
          <KeyValueList
            items={[
              { label: 'From', value: created.sender.city },
              { label: 'To', value: created.recipient.city },
            ]}
          />
          <FeeBreakdown fee={created.fee} />
          <div className="flex flex-wrap gap-3">
            <ButtonLink href={`/parcels/${created.id}`}>View parcel</ButtonLink>
            <ButtonLink href="/parcels" variant="secondary">
              My parcels
            </ButtonLink>
          </div>
        </Section>
      </>
    );

  return (
    <>
      <PageHeader
        title="New delivery"
        description="Arrange a pickup and see the delivery fee before you confirm."
      />
      {/* TODO(design-foundation): replace plain progress text when Stepper is available. */}
      <div ref={heading} tabIndex={-1}>
        <p
          className="text-overline uppercase text-text-muted"
          aria-live="polite"
        >
          Step {step + 1} of 3 ·{' '}
          {['Route and parcel', 'Contact details', 'Review delivery'][step]}
        </p>
      </div>
      <Section
        title={['Route and parcel', 'Contact details', 'Review delivery'][step]}
      >
        {problem !== null && <ParcelProblem error={problem} />}
        {isUncertain && (
          <Alert tone="warning" title="Your delivery may already be saved">
            Retry this unchanged request safely, or{' '}
            <a
              href="/parcels"
              className="text-interactive underline underline-offset-4"
            >
              check My parcels
            </a>{' '}
            before starting another delivery.
          </Alert>
        )}
        {zoneError !== null ? (
          <>
            <ParcelProblem error={zoneError} />
            <Button
              variant="secondary"
              onClick={() => {
                setZoneError(null);
                setReload((n) => n + 1);
              }}
            >
              Try again
            </Button>
          </>
        ) : zones === null ? (
          <SkeletonGroup label="Loading delivery zones">
            <SkeletonText />
          </SkeletonGroup>
        ) : zones.length === 0 ? (
          <EmptyState title="No delivery zones available">
            Please try again later.
          </EmptyState>
        ) : (
          <form
            noValidate
            onSubmit={
              step < 2
                ? next
                : (event) => {
                    event.preventDefault();
                    void submit();
                  }
            }
            className="flex flex-col gap-6"
          >
            {step === 0 && (
              <>
                <div className="grid gap-4 md:grid-cols-2">
                  {(['senderZone', 'recipientZone'] as const).map((field) => (
                    <Select
                      key={field}
                      id={`delivery-${field}`}
                      label={
                        field === 'senderZone'
                          ? 'Pickup zone'
                          : 'Destination zone'
                      }
                      placeholder="Choose a zone"
                      isRequired
                      options={zones.map((zone) => ({
                        value: zone.id,
                        label: zone.name,
                      }))}
                      value={values[field]}
                      error={form.errors[field] ?? estimateErrors[field]}
                      onChange={(event) => edit(field, event.target.value)}
                      onBlur={() => form.blur(field)}
                    />
                  ))}
                  <TextField
                    id="delivery-weight"
                    label="Weight"
                    isRequired
                    inputMode="decimal"
                    suffix="kg"
                    isMono
                    value={values.weight}
                    error={form.errors.weight ?? estimateErrors.weight}
                    onChange={(event) => edit('weight', event.target.value)}
                    onBlur={() => form.blur('weight')}
                  />
                  <Select
                    id="delivery-size"
                    label="Size category"
                    isRequired
                    options={sizeCategories.map((size) => ({
                      value: size,
                      label: size.charAt(0) + size.slice(1).toLowerCase(),
                    }))}
                    value={values.size}
                    error={form.errors.size ?? estimateErrors.size}
                    onChange={(event) => edit('size', event.target.value)}
                    onBlur={() => form.blur('size')}
                  />
                </div>
              </>
            )}
            {step === 1 && (
              <>
                <div className="grid gap-8 md:grid-cols-2">
                  <ContactFields
                    kind="sender"
                    values={values}
                    errors={form.errors}
                    onChange={edit}
                    onBlur={form.blur}
                  />
                  <ContactFields
                    kind="recipient"
                    values={values}
                    errors={form.errors}
                    onChange={edit}
                    onBlur={form.blur}
                  />
                </div>
                <Textarea
                  id="delivery-description"
                  label="Description (optional)"
                  maxLength={255}
                  showCount
                  value={values.description}
                  error={form.errors.description}
                  onChange={(event) => edit('description', event.target.value)}
                  onBlur={() => form.blur('description')}
                />
              </>
            )}
            {step === 2 && (
              <>
                <KeyValueList
                  layout="inline"
                  items={[
                    {
                      label: 'Sender',
                      value: `${values.senderName} · ${values.senderPhone}`,
                    },
                    {
                      label: 'Pickup address',
                      value: `${values.senderAddress}, ${values.senderCity}`,
                    },
                    {
                      label: 'Pickup zone',
                      value: zones.find((zone) => zone.id === values.senderZone)
                        ?.name,
                    },
                    {
                      label: 'Recipient',
                      value: `${values.recipientName} · ${values.recipientPhone}`,
                    },
                    {
                      label: 'Destination address',
                      value: `${values.recipientAddress}, ${values.recipientCity}`,
                    },
                    {
                      label: 'Destination zone',
                      value: zones.find(
                        (zone) => zone.id === values.recipientZone,
                      )?.name,
                    },
                    {
                      label: 'Weight',
                      value: formatKg(Number(values.weight)),
                      isMono: true,
                    },
                    { label: 'Size', value: values.size },
                    { label: 'Description', value: values.description },
                  ]}
                />
                <Alert tone="info" title="Before you confirm">
                  Check the addresses and phone numbers before creating your
                  delivery. The fee below is saved with your parcel.
                </Alert>
              </>
            )}
            <div aria-live="polite">
              {quote.status === 'ready' && quote.fingerprint === fingerprint ? (
                <FeeBreakdown fee={quote.fee} />
              ) : quote.status === 'error' &&
                Object.keys(estimateErrors).length > 0 ? (
                // The message is beside its field, and retrying would not help.
                <p className="text-body-sm text-text-muted">
                  Change the marked details to see a fee estimate.
                </p>
              ) : quote.status === 'error' ? (
                <>
                  <ParcelProblem error={quote.error} />
                  <Button
                    variant="secondary"
                    type="button"
                    isDisabled={estimateWait > 0}
                    onClick={() => {
                      if (estimateWait === 0) setReload((n) => n + 1);
                    }}
                  >
                    Retry estimate
                  </Button>
                </>
              ) : quote.status === 'loading' ? (
                <SkeletonGroup label="Calculating delivery fee">
                  <SkeletonText lines={2} />
                </SkeletonGroup>
              ) : (
                <p className="text-body-sm text-text-muted">
                  Choose both zones and enter the parcel weight to see a fee
                  estimate.
                </p>
              )}
            </div>
            <div className="flex flex-wrap gap-3">
              {step > 0 && (
                <Button
                  variant="secondary"
                  type="button"
                  isDisabled={isLoading || isUncertain}
                  onClick={() => go(step - 1)}
                >
                  Back
                </Button>
              )}
              <Button
                type="submit"
                isLoading={isLoading}
                isDisabled={
                  (step === 2 && createWait > 0) ||
                  quote.status !== 'ready' ||
                  quote.fingerprint !== fingerprint
                }
              >
                {isLoading
                  ? 'Creating delivery'
                  : step === 2
                    ? 'Create delivery'
                    : 'Continue'}
              </Button>
            </div>
          </form>
        )}
      </Section>
      <Dialog
        open={changedFee !== null}
        onOpenChange={(open) => {
          if (!open) setChangedFee(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>The delivery fee changed</DialogTitle>
            <DialogDescription>
              The current fee is {formatLkr(changedFee ?? 0)}. Refresh the
              breakdown and review it before creating your delivery.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="secondary" onClick={() => setChangedFee(null)}>
              Keep reviewing
            </Button>
            <Button onClick={() => void confirmPrice()}>Review new fee</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
