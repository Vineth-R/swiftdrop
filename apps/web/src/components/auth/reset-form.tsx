'use client';

import { Check, Clock } from 'lucide-react';
import { useRouter, useSearchParams } from 'next/navigation';
import { useEffect, useRef, useState, type FormEvent } from 'react';

import { ApiError } from '@/lib/api';
import { classifyApiError, type ApiProblem } from '@/lib/api-problem';
import { resetPassword } from '@/lib/auth-api';
import { RESET_REDIRECT_SECONDS } from '@/lib/constants';
import { useCountdown } from '@/lib/use-countdown';
import { useForm, type FormRules } from '@/lib/use-form';
import {
  serverFieldMessage,
  validateConfirmation,
  validateNewPassword,
} from '@/lib/validation';
import { AuthHeading } from '@/components/layout/auth-layout';
import { Alert } from '@/components/ui/alert';
import { Button, ButtonLink } from '@/components/ui/button';
import { PasswordField } from '@/components/ui/password-field';
import { RouteLine } from '@/components/ui/route-line';
import { ApiProblemAlerts } from './api-problem-alerts';
import { RequestResetForm } from './request-reset';

type Values = { password: string; confirm: string };

const rules: FormRules<Values> = {
  password: (v) => validateNewPassword(v.password),
  confirm: (v) => validateConfirmation(v.password, v.confirm),
};

export function ResetPasswordForm() {
  const router = useRouter();
  const token = useSearchParams().get('token') ?? '';

  const form = useForm({ password: '', confirm: '' }, rules);
  const [phase, setPhase] = useState<'form' | 'saved' | 'expired'>(
    token ? 'form' : 'expired',
  );
  const [isLoading, setIsLoading] = useState(false);
  const [problem, setProblem] = useState<ApiProblem | null>(null);
  const alertRef = useRef<HTMLDivElement>(null);

  const waiting = useCountdown(
    problem?.kind === 'rate-limited' ? problem.seconds : null,
  );
  const isWaiting = problem?.kind === 'rate-limited' && waiting > 0;

  const redirectIn = useCountdown(
    phase === 'saved' ? RESET_REDIRECT_SECONDS : null,
  );
  useEffect(() => {
    if (phase === 'saved' && redirectIn === 0) router.replace('/login');
  }, [phase, redirectIn, router]);

  useEffect(() => {
    if (problem) alertRef.current?.focus();
  }, [problem]);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isLoading || isWaiting) return;
    setProblem(null);

    const found = form.validate();
    if (found.password || found.confirm) {
      document
        .getElementById(found.password ? 'reset-password' : 'reset-confirm')
        ?.focus();
      return;
    }

    setIsLoading(true);
    try {
      await resetPassword(token, form.values.password);
      setPhase('saved');
    } catch (error) {
      setIsLoading(false);
      if (error instanceof ApiError && error.code === 'RESET_TOKEN_INVALID') {
        setPhase('expired');
        return;
      }
      const classified = classifyApiError(error);
      if (classified.kind === 'validation') {
        const message = classified.issues
          .map((issue) => serverFieldMessage(issue.field))
          .find(Boolean);
        if (message) form.setServerErrors({ password: message });
        else setProblem(classified);
      } else {
        setProblem(classified);
      }
    }
  }

  if (phase === 'expired') {
    return (
      <RequestResetForm
        title="Get a new link"
        intro="Enter the email on the account and we will send a fresh link."
        submitLabel="Send a new link"
        lead={
          <Alert
            tone="danger"
            icon={Clock}
            title="This reset link no longer works"
          >
            Links last 30 minutes and can only be used once. Ask for a new one
            and it will arrive in a moment.
          </Alert>
        }
      />
    );
  }

  if (phase === 'saved') {
    return (
      <div className="flex flex-col items-start gap-5">
        <span
          aria-hidden="true"
          className="inline-flex size-11 items-center justify-center rounded-full border border-success-border bg-success-tint text-success"
        >
          <Check size={24} strokeWidth={1.5} />
        </span>
        <div className="flex flex-col gap-2">
          <h1 className="font-display text-h2 md:text-h1">Password updated</h1>
          <p className="text-body text-text-muted">
            Log in with the new password. Every other session has been signed
            out, on every device.
          </p>
        </div>
        <RouteLine state="complete" className="w-full" />
        <ButtonLink href="/login" size="lg" isFullWidth>
          Go to log in
        </ButtonLink>
        <p role="status" className="tabular text-caption text-text-faint">
          Redirecting automatically in {redirectIn} seconds.
        </p>
      </div>
    );
  }

  return (
    <>
      <AuthHeading eyebrow="Account recovery" title="Set a new password">
        Choose a password you have not used before.
      </AuthHeading>
      <ApiProblemAlerts
        problem={problem}
        alertRef={alertRef}
        remaining={waiting}
      />
      <form noValidate onSubmit={submit} className="flex flex-col gap-5">
        <PasswordField
          id="reset-password"
          label="New password"
          autoComplete="new-password"
          controlSize="lg"
          showChecklist
          value={form.values.password}
          readOnly={isLoading}
          error={form.errors.password}
          onChange={(event) => form.setValue('password', event.target.value)}
          onBlur={() => form.blur('password')}
        />
        <PasswordField
          id="reset-confirm"
          label="Confirm new password"
          autoComplete="new-password"
          controlSize="lg"
          value={form.values.confirm}
          readOnly={isLoading}
          error={form.errors.confirm}
          onChange={(event) => form.setValue('confirm', event.target.value)}
          onBlur={() => form.blur('confirm')}
        />
        <Button
          type="submit"
          size="lg"
          isFullWidth
          isLoading={isLoading}
          isDisabled={isWaiting}
        >
          {isLoading ? 'Saving' : 'Save new password'}
        </Button>
        <p className="text-caption text-text-faint">
          Saving signs you out of every other session on every device.
        </p>
      </form>
    </>
  );
}
