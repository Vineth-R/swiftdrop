'use client';

import { ChevronLeft } from 'lucide-react';
import Link from 'next/link';
import {
  useEffect,
  useRef,
  useState,
  type FormEvent,
  type ReactNode,
} from 'react';

import { classifyApiError, type ApiProblem } from '@/lib/api-problem';
import { forgotPassword } from '@/lib/auth-api';
import { useCountdown } from '@/lib/use-countdown';
import { useForm, type FormRules } from '@/lib/use-form';
import { validateEmail } from '@/lib/validation';
import { AuthHeading } from '@/components/layout/auth-layout';
import { Button, ButtonLink } from '@/components/ui/button';
import { TextField } from '@/components/ui/field';
import { RouteLine } from '@/components/ui/route-line';
import { ApiProblemAlerts } from './api-problem-alerts';

const rules: FormRules<{ email: string }> = {
  email: (v) => validateEmail(v.email),
};

type Props = {
  title?: string;
  intro?: ReactNode;
  /** Shown above the form, such as "this link no longer works". */
  lead?: ReactNode;
  submitLabel?: string;
};

/**
 * Asks for a reset link. The reply is always the same, whether or not the email
 * has an account, and so is the copy: that is what stops this page from being
 * used to find out which emails are registered.
 */
export function RequestResetForm({
  title = 'Forgot your password?',
  intro = 'Enter the email on the account and we will send a link to set a new password.',
  lead,
  submitLabel = 'Send reset link',
}: Props) {
  const form = useForm({ email: '' }, rules);
  const [sentTo, setSentTo] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [problem, setProblem] = useState<ApiProblem | null>(null);
  const [resent, setResent] = useState(false);
  const alertRef = useRef<HTMLDivElement>(null);

  const waiting = useCountdown(
    problem?.kind === 'rate-limited' ? problem.seconds : null,
  );
  const isWaiting = problem?.kind === 'rate-limited' && waiting > 0;

  useEffect(() => {
    if (problem) alertRef.current?.focus();
  }, [problem]);

  async function request(email: string): Promise<boolean> {
    setProblem(null);
    setIsLoading(true);
    try {
      await forgotPassword(email);
      return true;
    } catch (error) {
      const classified = classifyApiError(error);
      if (classified.kind === 'validation') {
        form.setServerErrors({ email: 'Enter a complete email address.' });
      } else {
        setProblem(classified);
      }
      return false;
    } finally {
      setIsLoading(false);
    }
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isLoading || isWaiting) return;
    if (form.validate().email) {
      document.getElementById('forgot-email')?.focus();
      return;
    }
    if (await request(form.values.email)) setSentTo(form.values.email.trim());
  }

  async function sendAgain() {
    if (sentTo && (await request(sentTo))) setResent(true);
  }

  if (sentTo) {
    return (
      <div className="flex flex-col gap-6">
        <RouteLine state="complete" />
        <div className="flex flex-col gap-2">
          <span className="text-overline uppercase text-text-faint">
            Account recovery
          </span>
          <h1 className="font-display text-h2 md:text-h1">Check your email</h1>
          <p className="text-body text-text-muted">
            If an account exists for this email, a reset link is on its way. The
            link is valid for 30 minutes and can be used once.
          </p>
        </div>
        <div className="flex flex-col gap-1 rounded-control bg-surface-2 px-4 py-3">
          <span className="text-overline uppercase text-text-faint">
            Sent to
          </span>
          <span className="break-all font-mono text-body-sm">{sentTo}</span>
        </div>
        <ApiProblemAlerts
          problem={problem}
          alertRef={alertRef}
          remaining={waiting}
        />
        <div className="flex flex-col gap-3">
          <ButtonLink href="/login" size="lg" isFullWidth>
            Back to log in
          </ButtonLink>
          <p className="text-body-sm text-text-muted">
            Nothing after a few minutes? Check spam, then{' '}
            <Button
              variant="link"
              isLoading={isLoading}
              isDisabled={isWaiting}
              onClick={sendAgain}
            >
              send it again
            </Button>
            .
          </p>
          {resent && (
            <p role="status" className="text-caption text-text-faint">
              We sent it again.
            </p>
          )}
        </div>
      </div>
    );
  }

  return (
    <>
      <AuthHeading eyebrow="Account recovery" title={title}>
        {intro}
      </AuthHeading>
      {lead}
      <ApiProblemAlerts
        problem={problem}
        alertRef={alertRef}
        remaining={waiting}
      />
      <form noValidate onSubmit={submit} className="flex flex-col gap-5">
        <TextField
          id="forgot-email"
          label="Email"
          type="email"
          autoComplete="email"
          placeholder="you@example.lk"
          controlSize="lg"
          value={form.values.email}
          readOnly={isLoading}
          error={form.errors.email}
          onChange={(event) => form.setValue('email', event.target.value)}
          onBlur={() => form.blur('email')}
        />
        <Button
          type="submit"
          size="lg"
          isFullWidth
          isLoading={isLoading}
          isDisabled={isWaiting}
        >
          {isLoading ? 'Sending' : submitLabel}
        </Button>
      </form>
      <Link
        href="/login"
        className="inline-flex w-fit items-center gap-2 text-label font-semibold text-interactive hover:text-interactive-hover"
      >
        <ChevronLeft size={16} strokeWidth={1.5} aria-hidden="true" />
        Back to log in
      </Link>
    </>
  );
}
