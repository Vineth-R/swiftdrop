'use client';

import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { useEffect, useRef, useState, type FormEvent } from 'react';

import { ApiError } from '@/lib/api';
import { classifyApiError, type ApiProblem } from '@/lib/api-problem';
import { googleLoginUrl, register } from '@/lib/auth-api';
import { DEFAULT_AUTH_ROUTE } from '@/lib/constants';
import { safeNext } from '@/lib/redirect';
import { formatClock, useCountdown } from '@/lib/use-countdown';
import { useForm, type FormRules } from '@/lib/use-form';
import {
  serverFieldMessage,
  toSriLankanE164,
  validateConfirmation,
  validateEmail,
  validateFullName,
  validateNewPassword,
  validatePhone,
} from '@/lib/validation';
import { Alert } from '@/components/ui/alert';
import { Button, ButtonLink } from '@/components/ui/button';
import {
  ErrorSummary,
  type ErrorSummaryIssue,
} from '@/components/ui/error-summary';
import { TextField } from '@/components/ui/field';
import { PasswordField } from '@/components/ui/password-field';
import { ApiProblemAlerts } from './api-problem-alerts';
import { GoogleButton, OrDivider } from './google-button';

type Values = {
  fullName: string;
  email: string;
  phone: string;
  password: string;
  confirm: string;
};
type Field = keyof Values;

/**
 * Client-only marker for the email field. It is deliberately not the API's
 * EMAIL_ALREADY_EXISTS code: transport vocabulary stays in the API layer.
 */
const EMAIL_IN_USE_FIELD = 'client.email-in-use';

const rules: FormRules<Values> = {
  fullName: (v) => validateFullName(v.fullName),
  email: (v) => validateEmail(v.email),
  phone: (v) => validatePhone(v.phone),
  password: (v) => validateNewPassword(v.password),
  confirm: (v) => validateConfirmation(v.password, v.confirm),
};

const labels: Record<Field, string> = {
  fullName: 'Full name',
  email: 'Email',
  phone: 'Phone',
  password: 'Password',
  confirm: 'Confirm password',
};

const fieldId = (field: Field) => `register-${field}`;

type Banner = ApiProblem;

export function RegisterForm() {
  const params = useSearchParams();
  const next = safeNext(params.get('next'), '');

  const form = useForm(
    { fullName: '', email: '', phone: '', password: '', confirm: '' },
    rules,
  );
  const [banner, setBanner] = useState<Banner | null>(null);
  const [failedSubmits, setFailedSubmits] = useState(0);
  const [isLoading, setIsLoading] = useState(false);
  const [createdEmail, setCreatedEmail] = useState<string | null>(null);
  const alertRef = useRef<HTMLDivElement>(null);

  const waitSeconds = banner?.kind === 'rate-limited' ? banner.seconds : null;
  const waiting = useCountdown(waitSeconds);
  const isWaiting = banner?.kind === 'rate-limited' && waiting > 0;

  useEffect(() => {
    if (banner) alertRef.current?.focus();
  }, [banner]);

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

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isLoading || isWaiting) return;
    setBanner(null);

    if (Object.keys(form.validate()).length > 0) {
      setFailedSubmits((count) => count + 1);
      return;
    }

    setIsLoading(true);
    try {
      await register({
        fullName: form.values.fullName,
        email: form.values.email,
        phone: toSriLankanE164(form.values.phone),
        password: form.values.password,
      });
      setCreatedEmail(form.values.email.trim());
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

    const classified = classifyApiError(error);
    if (classified.kind === 'validation') {
      const fields: Partial<Record<Field, string>> = {};
      for (const issue of classified.issues) {
        const message = serverFieldMessage(issue.field);
        if (message && issue.field in labels)
          fields[issue.field as Field] = message;
      }
      if (Object.keys(fields).length > 0) {
        form.setServerErrors(fields);
        setFailedSubmits((count) => count + 1);
        return;
      }
    }
    setBanner(classified);
  }

  if (createdEmail) {
    return (
      <div className="flex flex-col gap-4">
        <Alert tone="success" title="Check your email">
          We sent a verification link to{' '}
          <span className="font-mono text-text">{createdEmail}</span>. Open the
          link within 30 minutes to activate your account.
        </Alert>
        <p className="text-body-sm text-text-muted">
          You cannot sign in until your email is verified. If the email does not
          arrive, use the verification page to request another one.
        </p>
        <ButtonLink href="/verify-email" size="lg" isFullWidth>
          Open verification page
        </ButtonLink>
        <ButtonLink href="/login" variant="secondary" isFullWidth>
          Back to log in
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

  return (
    <>
      {banner && (
        <ApiProblemAlerts
          problem={banner}
          alertRef={alertRef}
          remaining={waiting}
        />
      )}
      {issues.length > 0 && failedSubmits > 0 && (
        <ErrorSummary key={failedSubmits} issues={issues} />
      )}

      <form noValidate onSubmit={submit} className="flex flex-col gap-5">
        <TextField
          id={fieldId('fullName')}
          label="Full name"
          autoComplete="name"
          controlSize="lg"
          isRequired
          helper="2 to 150 characters. This is the name couriers will see."
          value={form.values.fullName}
          readOnly={isLoading}
          error={form.errors.fullName}
          onChange={(event) => form.setValue('fullName', event.target.value)}
          onBlur={() => form.blur('fullName')}
        />
        <TextField
          id={fieldId('email')}
          label="Email"
          type="email"
          autoComplete="email"
          controlSize="lg"
          isRequired
          value={form.values.email}
          readOnly={isLoading}
          error={emailError}
          onChange={(event) => form.setValue('email', event.target.value)}
          onBlur={() => form.blur('email')}
        />
        <TextField
          id={fieldId('phone')}
          label="Phone"
          type="tel"
          autoComplete="tel-national"
          inputMode="tel"
          controlSize="lg"
          isRequired
          isMono
          prefix="+94"
          helper="Used for delivery calls."
          value={form.values.phone}
          readOnly={isLoading}
          error={form.errors.phone}
          onChange={(event) => form.setValue('phone', event.target.value)}
          onBlur={() => form.blur('phone')}
        />
        <PasswordField
          id={fieldId('password')}
          label="Password"
          autoComplete="new-password"
          controlSize="lg"
          isRequired
          showChecklist
          value={form.values.password}
          readOnly={isLoading}
          error={form.errors.password}
          onChange={(event) => form.setValue('password', event.target.value)}
          onBlur={() => form.blur('password')}
        />
        <PasswordField
          id={fieldId('confirm')}
          label="Confirm password"
          autoComplete="new-password"
          controlSize="lg"
          isRequired
          value={form.values.confirm}
          readOnly={isLoading}
          error={form.errors.confirm}
          onChange={(event) => form.setValue('confirm', event.target.value)}
          onBlur={() => form.blur('confirm')}
        />
        <p className="text-caption text-text-muted">
          By creating an account you agree to the Terms of service and the
          Privacy policy. We will email you about your own parcels only.
        </p>
        <Button
          type="submit"
          size="lg"
          isFullWidth
          isLoading={isLoading}
          isDisabled={isWaiting}
          className="tabular"
        >
          {isLoading
            ? 'Creating account'
            : isWaiting
              ? `Try again in ${formatClock(waiting)}`
              : 'Create account'}
        </Button>
      </form>

      <OrDivider />
      <GoogleButton href={googleLoginUrl(next || DEFAULT_AUTH_ROUTE)}>
        Sign up with Google
      </GoogleButton>
    </>
  );
}
