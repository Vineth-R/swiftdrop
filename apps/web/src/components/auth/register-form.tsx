'use client';

import Link from 'next/link';
import { useRouter, useSearchParams } from 'next/navigation';
import { useEffect, useRef, useState, type FormEvent } from 'react';

import { ApiError } from '../../lib/api';
import { googleLoginUrl, register } from '../../lib/auth-api';
import { homeFor } from '../../lib/nav';
import { safeNext } from '../../lib/redirect';
import { useSession } from '../../lib/session';
import { formatClock, useCountdown } from '../../lib/use-countdown';
import { useForm, type FormRules } from '../../lib/use-form';
import {
  serverFieldMessage,
  toSriLankanE164,
  validateConfirmation,
  validateEmail,
  validateFullName,
  validateNewPassword,
  validatePhone,
} from '../../lib/validation';
import { Alert } from '../ui/alert';
import { Button, ButtonLink } from '../ui/button';
import { ErrorSummary, type ErrorSummaryIssue } from '../ui/error-summary';
import { TextField } from '../ui/field';
import { PasswordField } from '../ui/password-field';
import { RateLimitedAlert, ServerFaultAlert } from './form-alerts';
import { GoogleButton, OrDivider } from './google-button';

type Values = {
  fullName: string;
  email: string;
  phone: string;
  password: string;
  confirm: string;
};
type Field = keyof Values;

/** Stands in for the "already registered" message, which carries links. */
const EMAIL_IN_USE = 'EMAIL_IN_USE';

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

type Banner =
  | { kind: 'rate-limited'; seconds: number }
  | { kind: 'server'; requestId?: string; isNetwork: boolean }
  | { kind: 'signed-out-after-create' };

export function RegisterForm() {
  const router = useRouter();
  const params = useSearchParams();
  const { signIn } = useSession();
  const next = safeNext(params.get('next'), '');

  const form = useForm(
    { fullName: '', email: '', phone: '', password: '', confirm: '' },
    rules,
  );
  const [banner, setBanner] = useState<Banner | null>(null);
  const [failedSubmits, setFailedSubmits] = useState(0);
  const [isLoading, setIsLoading] = useState(false);
  const [createdAs, setCreatedAs] = useState<{
    email: string;
    to: string;
  } | null>(null);
  const alertRef = useRef<HTMLDivElement>(null);

  const waitSeconds = banner?.kind === 'rate-limited' ? banner.seconds : null;
  const waiting = useCountdown(waitSeconds);
  const isWaiting = banner?.kind === 'rate-limited' && waiting > 0;

  const redirectIn = useCountdown(createdAs ? 3 : null);
  useEffect(() => {
    if (createdAs && redirectIn === 0) router.replace(createdAs.to);
  }, [createdAs, redirectIn, router]);

  useEffect(() => {
    if (banner) alertRef.current?.focus();
  }, [banner]);

  const issues: ErrorSummaryIssue[] = (Object.keys(labels) as Field[])
    .filter((field) => form.errors[field])
    .map((field) => ({
      fieldId: fieldId(field),
      label: labels[field],
      message:
        form.errors[field] === EMAIL_IN_USE
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
    } catch (error) {
      setIsLoading(false);
      handleFailure(error);
      return;
    }

    try {
      const user = await signIn(form.values.email, form.values.password);
      setCreatedAs({
        email: user.email,
        to: safeNext(next, homeFor(user.role)),
      });
    } catch {
      setIsLoading(false);
      setBanner({ kind: 'signed-out-after-create' });
    }
  }

  function handleFailure(error: unknown) {
    if (!(error instanceof ApiError)) {
      setBanner({ kind: 'server', isNetwork: false });
      return;
    }
    switch (error.code) {
      case 'EMAIL_IN_USE':
        form.setServerErrors({ email: EMAIL_IN_USE });
        setFailedSubmits((count) => count + 1);
        break;
      case 'VALIDATION_FAILED': {
        const fields: Partial<Record<Field, string>> = {};
        for (const issue of error.issues) {
          const message = serverFieldMessage(issue.field);
          if (message && issue.field in labels)
            fields[issue.field as Field] = message;
        }
        if (Object.keys(fields).length > 0) {
          form.setServerErrors(fields);
          setFailedSubmits((count) => count + 1);
        } else {
          setBanner({ kind: 'server', isNetwork: false });
        }
        break;
      }
      case 'RATE_LIMITED':
        setBanner({ kind: 'rate-limited', seconds: error.retryAfter ?? 60 });
        break;
      default:
        setBanner({
          kind: 'server',
          isNetwork: error.code === 'NETWORK',
          requestId: error.code === 'INTERNAL' ? error.requestId : undefined,
        });
    }
  }

  if (createdAs) {
    return (
      <div className="flex flex-col gap-4">
        <Alert tone="success" title="Account created">
          You are signed in as{' '}
          <span className="font-mono text-text">{createdAs.email}</span>.
        </Alert>
        <ButtonLink href={createdAs.to} size="lg" isFullWidth>
          Go to my dashboard
        </ButtonLink>
        <p role="status" className="tabular text-caption text-text-faint">
          Taking you there in {redirectIn} seconds.
        </p>
      </div>
    );
  }

  const emailError =
    form.errors.email === EMAIL_IN_USE ? (
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
      {banner?.kind === 'rate-limited' && (
        <RateLimitedAlert alertRef={alertRef} remaining={waiting} />
      )}
      {banner?.kind === 'server' && (
        <ServerFaultAlert
          alertRef={alertRef}
          isNetwork={banner.isNetwork}
          requestId={banner.requestId}
        />
      )}
      {banner?.kind === 'signed-out-after-create' && (
        <Alert ref={alertRef} tone="warning" title="Your account was created">
          We could not sign you in just now.{' '}
          <Link
            href="/login"
            className="font-semibold underline underline-offset-4"
          >
            Log in
          </Link>{' '}
          with the email and password you chose.
        </Alert>
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
      <GoogleButton href={googleLoginUrl(next || '/dashboard')}>
        Sign up with Google
      </GoogleButton>
    </>
  );
}
