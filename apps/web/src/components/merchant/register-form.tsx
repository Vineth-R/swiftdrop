'use client';

import Link from 'next/link';
import { useEffect, useRef, useState, type FormEvent } from 'react';

import { ApiError } from '@/lib/api';
import { classifyApiError, type ApiProblem } from '@/lib/api-problem';
import { registerMerchant } from '@/lib/merchant-api';
import { getZones, type Zone } from '@/lib/pricing-api';
import { formatClock, useCountdown } from '@/lib/use-countdown';
import { useForm, type FormRules } from '@/lib/use-form';
import {
  serverFieldMessage,
  toSriLankanE164,
  validateAddress,
  validateBusinessName,
  validateConfirmation,
  validateEmail,
  validateFullName,
  validateIfFilled,
  validateNewPassword,
  validatePhone,
  validateRegistrationNumber,
} from '@/lib/validation';
import { ApiProblemAlerts } from '../auth/api-problem-alerts';
import { Alert } from '../ui/alert';
import { Button, ButtonLink } from '../ui/button';
import { ErrorSummary, type ErrorSummaryIssue } from '../ui/error-summary';
import { TextField } from '../ui/field';
import { PasswordField } from '../ui/password-field';
import { Select } from '../ui/select';

type Values = {
  fullName: string;
  email: string;
  phone: string;
  password: string;
  confirm: string;
  businessName: string;
  registrationNumber: string;
  contactEmail: string;
  contactPhone: string;
  businessAddress: string;
  pickupAddress: string;
  pickupZoneId: string;
};
type Field = keyof Values;

const empty: Values = {
  fullName: '',
  email: '',
  phone: '',
  password: '',
  confirm: '',
  businessName: '',
  registrationNumber: '',
  contactEmail: '',
  contactPhone: '',
  businessAddress: '',
  pickupAddress: '',
  pickupZoneId: '',
};

/**
 * Client-only marker for the email field, so the transport code
 * (EMAIL_ALREADY_EXISTS) stays in the API layer.
 */
const EMAIL_IN_USE_FIELD = 'client.email-in-use';

// Defined once, outside the component, so useForm keeps the same object.
const rules: FormRules<Values> = {
  fullName: (v) => validateFullName(v.fullName),
  email: (v) => validateEmail(v.email),
  phone: (v) => validatePhone(v.phone),
  password: (v) => validateNewPassword(v.password),
  confirm: (v) => validateConfirmation(v.password, v.confirm),
  businessName: (v) => validateBusinessName(v.businessName),
  registrationNumber: (v) => validateRegistrationNumber(v.registrationNumber),
  contactEmail: (v) => validateIfFilled(v.contactEmail, validateEmail),
  contactPhone: (v) => validateIfFilled(v.contactPhone, validatePhone),
  businessAddress: (v) => validateAddress(v.businessAddress),
  pickupAddress: (v) => validateAddress(v.pickupAddress),
  pickupZoneId: (v) => (v.pickupZoneId ? undefined : 'Choose a pickup zone.'),
};

const labels: Record<Field, string> = {
  fullName: 'Full name',
  email: 'Email',
  phone: 'Phone number',
  password: 'Password',
  confirm: 'Confirm password',
  businessName: 'Business name',
  registrationNumber: 'Business registration number',
  contactEmail: 'Business email',
  contactPhone: 'Business phone',
  businessAddress: 'Business address',
  pickupAddress: 'Default pickup address',
  pickupZoneId: 'Default pickup zone',
};

/** The API's names for the fields it can reject (API 8.1), in our own. */
const serverFields: Record<string, Field> = {
  fullName: 'fullName',
  email: 'email',
  phone: 'phone',
  password: 'password',
  'business.name': 'businessName',
  'business.registrationNumber': 'registrationNumber',
  'business.contactEmail': 'contactEmail',
  'business.contactPhone': 'contactPhone',
  'business.businessAddress': 'businessAddress',
  'business.pickupAddress': 'pickupAddress',
  'business.pickupZoneId': 'pickupZoneId',
};

const fieldId = (field: Field) => `merchant-${field}`;

/** An application already exists, and the API cannot say whether by email or by number. */
type Banner = ApiProblem | { kind: 'duplicate' };

export function MerchantRegisterForm() {
  const form = useForm(empty, rules);
  const [zones, setZones] = useState<Zone[]>([]);
  const [zonesLoading, setZonesLoading] = useState(true);
  const [zonesError, setZonesError] = useState(false);
  const [revision, setRevision] = useState(0);
  const [banner, setBanner] = useState<Banner | null>(null);
  const [failedSubmits, setFailedSubmits] = useState(0);
  const [isLoading, setIsLoading] = useState(false);
  const [success, setSuccess] = useState<{ merchantId: string } | null>(null);
  const alertRef = useRef<HTMLDivElement>(null);
  // A retry of the same application reuses one key, so a lost reply cannot
  // create it twice. Changing what is sent starts a new application, so it gets
  // a new key: the API ties a key to the email it first saw.
  const attempt = useRef<{ signature: string; key: string } | null>(null);

  const waitSeconds = banner?.kind === 'rate-limited' ? banner.seconds : null;
  const waiting = useCountdown(waitSeconds);
  const isWaiting = banner?.kind === 'rate-limited' && waiting > 0;

  useEffect(() => {
    if (banner) alertRef.current?.focus();
  }, [banner]);

  useEffect(() => {
    const controller = new AbortController();
    setZonesLoading(true);
    setZonesError(false);
    getZones(controller.signal)
      .then((data) => {
        if (!controller.signal.aborted) setZones(data);
      })
      .catch(() => {
        if (!controller.signal.aborted) setZonesError(true);
      })
      .finally(() => {
        if (!controller.signal.aborted) setZonesLoading(false);
      });
    return () => controller.abort();
  }, [revision]);

  const issues: ErrorSummaryIssue[] = (Object.keys(labels) as Field[])
    .filter((field) => form.errors[field])
    .map((field) => ({
      fieldId: fieldId(field),
      label: labels[field],
      message:
        form.errors[field] === EMAIL_IN_USE_FIELD
          ? 'this email is already registered.'
          : (form.errors[field] ?? '').replace(/^./, (c) => c.toLowerCase()),
    }));

  function idempotencyKeyFor(payload: unknown): string {
    const signature = JSON.stringify(payload);
    if (attempt.current?.signature !== signature) {
      attempt.current = { signature, key: crypto.randomUUID() };
    }
    return attempt.current.key;
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isLoading || isWaiting || zonesLoading || zonesError) return;
    setBanner(null);

    if (Object.keys(form.validate()).length > 0) {
      setFailedSubmits((count) => count + 1);
      return;
    }

    const values = form.values;
    const phone = toSriLankanE164(values.phone);
    const payload = {
      fullName: values.fullName.trim(),
      email: values.email.trim(),
      phone,
      password: values.password,
      business: {
        name: values.businessName.trim(),
        registrationNumber: values.registrationNumber.trim(),
        contactEmail: values.contactEmail.trim() || values.email.trim(),
        contactPhone: values.contactPhone.trim()
          ? toSriLankanE164(values.contactPhone)
          : phone,
        businessAddress: values.businessAddress.trim(),
        pickupAddress: values.pickupAddress.trim(),
        pickupZoneId: values.pickupZoneId,
      },
    };

    setIsLoading(true);
    try {
      const result = await registerMerchant(
        payload,
        idempotencyKeyFor(payload),
      );
      setSuccess({ merchantId: result.merchantId });
    } catch (error) {
      handleFailure(error);
    } finally {
      setIsLoading(false);
    }
  }

  function handleFailure(error: unknown) {
    if (error instanceof ApiError && error.code === 'EMAIL_IN_USE') {
      form.setServerErrors({ email: EMAIL_IN_USE_FIELD });
      setFailedSubmits((count) => count + 1);
      return;
    }
    if (
      error instanceof ApiError &&
      error.code === 'DUPLICATE_MERCHANT_APPLICATION'
    ) {
      setBanner({ kind: 'duplicate' });
      return;
    }

    const classified = classifyApiError(error);
    if (classified.kind === 'validation') {
      const fields: Partial<Record<Field, string>> = {};
      for (const issue of classified.issues) {
        const field = serverFields[issue.field];
        const message = serverFieldMessage(issue.field);
        if (field && message) fields[field] = message;
      }
      if (Object.keys(fields).length > 0) {
        form.setServerErrors(fields);
        setFailedSubmits((count) => count + 1);
        return;
      }
    }
    setBanner(classified);
  }

  if (success) {
    return (
      <div className="flex flex-col gap-5">
        <Alert tone="success" title="Application submitted">
          Your business application is awaiting administrator review. We will
          email you when a decision is made. You cannot sign in until the
          application is approved.
        </Alert>
        <p className="text-caption text-text-muted">
          Application reference:{' '}
          <span className="break-all font-mono text-text">
            {success.merchantId}
          </span>
        </p>
        <ButtonLink href="/merchant/pending" size="lg" isFullWidth>
          View application status
        </ButtonLink>
        <ButtonLink href="/login" variant="secondary" isFullWidth>
          Go to log in
        </ButtonLink>
      </div>
    );
  }

  const emailError =
    form.errors.email === EMAIL_IN_USE_FIELD ? (
      <>
        This email is already registered.{' '}
        <Link
          href="/login"
          className="font-semibold underline underline-offset-4"
        >
          Log in instead
        </Link>
        , or{' '}
        <Link
          href="/forgot-password"
          className="font-semibold underline underline-offset-4"
        >
          reset the password
        </Link>
        .
      </>
    ) : (
      form.errors.email
    );

  const text = (field: Field) => ({
    id: fieldId(field),
    label: labels[field],
    controlSize: 'lg' as const,
    value: form.values[field],
    readOnly: isLoading,
    error: form.errors[field],
    onChange: (event: { target: { value: string } }) =>
      form.setValue(field, event.target.value),
    onBlur: () => form.blur(field),
  });

  return (
    <>
      {banner?.kind === 'duplicate' ? (
        <Alert
          ref={alertRef}
          tone="danger"
          title="An application already exists"
        >
          A merchant application already exists for this email or business
          registration number. Check the details you entered, or{' '}
          <Link
            href="/login"
            className="font-semibold underline underline-offset-4"
          >
            log in
          </Link>{' '}
          to see where it stands.
        </Alert>
      ) : banner ? (
        <ApiProblemAlerts
          problem={banner}
          alertRef={alertRef}
          remaining={waiting}
        />
      ) : null}
      {issues.length > 0 && failedSubmits > 0 && (
        <ErrorSummary key={failedSubmits} issues={issues} />
      )}

      <form noValidate onSubmit={submit} className="flex flex-col gap-6">
        <fieldset className="flex flex-col gap-4">
          <legend className="mb-2 font-display text-h3">Your account</legend>
          <TextField
            {...text('fullName')}
            autoComplete="name"
            isRequired
            maxLength={150}
          />
          <div className="grid gap-4 sm:grid-cols-2">
            <TextField
              {...text('email')}
              error={emailError}
              type="email"
              autoComplete="email"
              isRequired
              maxLength={255}
            />
            <TextField
              {...text('phone')}
              type="tel"
              autoComplete="tel-national"
              inputMode="tel"
              isRequired
              isMono
              prefix="+94"
              helper="Used to reach you about your application."
            />
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <PasswordField
              {...text('password')}
              autoComplete="new-password"
              isRequired
              showChecklist
            />
            <PasswordField
              {...text('confirm')}
              autoComplete="new-password"
              isRequired
            />
          </div>
        </fieldset>

        <fieldset className="flex flex-col gap-4 border-t border-line pt-6">
          <legend className="mb-2 font-display text-h3">Your business</legend>
          <TextField
            {...text('businessName')}
            autoComplete="organization"
            isRequired
            maxLength={150}
          />
          <TextField
            {...text('registrationNumber')}
            isRequired
            isMono
            maxLength={50}
            helper="Required. For example PV 123456. Letters, digits, spaces, hyphens, slashes and dots are accepted."
          />
          <div className="grid gap-4 sm:grid-cols-2">
            <TextField
              {...text('contactEmail')}
              type="email"
              maxLength={255}
              helper="Defaults to your account email."
            />
            <TextField
              {...text('contactPhone')}
              type="tel"
              inputMode="tel"
              isMono
              prefix="+94"
              helper="Defaults to your account phone."
            />
          </div>
          <TextField
            {...text('businessAddress')}
            autoComplete="street-address"
            isRequired
            maxLength={255}
          />
          <TextField
            {...text('pickupAddress')}
            autoComplete="street-address"
            isRequired
            maxLength={255}
          />
          <Select
            id={fieldId('pickupZoneId')}
            label={labels.pickupZoneId}
            controlSize="lg"
            isRequired
            value={form.values.pickupZoneId}
            placeholder={
              zonesLoading ? 'Loading zones…' : 'Choose your pickup zone'
            }
            options={zones.map((zone) => ({
              value: zone.id,
              label: zone.name,
            }))}
            disabled={zonesLoading || zonesError || isLoading}
            error={form.errors.pickupZoneId}
            onChange={(event) =>
              form.setValue('pickupZoneId', event.target.value)
            }
            onBlur={() => form.blur('pickupZoneId')}
          />
        </fieldset>

        {zonesError ? (
          <Alert
            tone="danger"
            title="Could not load pickup zones"
            actions={
              <Button
                variant="secondary"
                onClick={() => setRevision((n) => n + 1)}
              >
                Retry zones
              </Button>
            }
          >
            Please retry before registering.
          </Alert>
        ) : !zonesLoading && !zones.length ? (
          <Alert tone="warning" title="No pickup zones available">
            Please try again later.
          </Alert>
        ) : null}

        <p className="text-caption text-text-muted">
          An administrator reviews every business. You cannot sign in until the
          application is approved.
        </p>
        <Button
          type="submit"
          size="lg"
          isFullWidth
          isLoading={isLoading}
          isDisabled={isWaiting || zonesLoading || zonesError || !zones.length}
          className="tabular"
        >
          {isLoading
            ? 'Submitting application'
            : isWaiting
              ? `Try again in ${formatClock(waiting)}`
              : 'Submit application'}
        </Button>
        <p className="text-center text-body-sm text-text-muted">
          Already registered?{' '}
          <Link
            href="/login"
            className="text-interactive underline underline-offset-4"
          >
            Log in
          </Link>
        </p>
      </form>
    </>
  );
}
