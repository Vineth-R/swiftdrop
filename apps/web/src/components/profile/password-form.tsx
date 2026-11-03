'use client';

import { Check, KeyRound } from 'lucide-react';
import { useRouter } from 'next/navigation';
import { useEffect, useRef, useState, type FormEvent } from 'react';

import { ApiError } from '../../lib/api';
import { changePassword, type User } from '../../lib/auth-api';
import { useSession } from '../../lib/session';
import { useCountdown } from '../../lib/use-countdown';
import { useForm, type FormRules } from '../../lib/use-form';
import {
  serverFieldMessage,
  validateConfirmation,
  validateCurrentPassword,
  validateNewPassword,
} from '../../lib/validation';
import { RateLimitedAlert, ServerFaultAlert } from '../auth/form-alerts';
import { Alert } from '../ui/alert';
import { Button, ButtonLink } from '../ui/button';
import { PasswordField } from '../ui/password-field';
import { RouteLine } from '../ui/route-line';

type Values = { current: string; password: string; confirm: string };

const rules: FormRules<Values> = {
  current: (v) => validateCurrentPassword(v.current),
  password: (v) => validateNewPassword(v.password),
  confirm: (v) => validateConfirmation(v.password, v.confirm),
};

const focusOrder = [
  ['current', 'security-current'],
  ['password', 'security-password'],
  ['confirm', 'security-confirm'],
] as const;

type Problem =
  | { kind: 'rate-limited'; seconds: number }
  | { kind: 'server'; isNetwork: boolean; requestId?: string };

/** What the person sees when there is no password to change. */
function GoogleOnly() {
  return (
    <Alert
      tone="info"
      icon={KeyRound}
      title="This account signs in with Google"
      actions={
        <ButtonLink href="/profile" variant="secondary" size="sm">
          Back to profile
        </ButtonLink>
      }
    >
      There is no SwiftDrop password to change. Manage how you sign in from your
      Google account.
    </Alert>
  );
}

export function PasswordForm({ user }: { user: User }) {
  const router = useRouter();
  const { recheck } = useSession();
  const form = useForm<Values>(
    { current: '', password: '', confirm: '' },
    rules,
  );
  const [phase, setPhase] = useState<'form' | 'saved' | 'google'>(
    user.authProvider === 'GOOGLE' ? 'google' : 'form',
  );
  const [isLoading, setIsLoading] = useState(false);
  const [problem, setProblem] = useState<Problem | null>(null);
  const alertRef = useRef<HTMLDivElement>(null);

  const waiting = useCountdown(
    problem?.kind === 'rate-limited' ? problem.seconds : null,
  );
  const isWaiting = problem?.kind === 'rate-limited' && waiting > 0;

  // The API ends every session when the password changes, this one included,
  // so the only way on is the login page.
  const redirectIn = useCountdown(phase === 'saved' ? 5 : null);
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
    const firstBad = focusOrder.find(([key]) => found[key]);
    if (firstBad) {
      document.getElementById(firstBad[1])?.focus();
      return;
    }

    setIsLoading(true);
    try {
      await changePassword(form.values.current, form.values.password);
      setPhase('saved');
    } catch (error) {
      setIsLoading(false);
      if (!(error instanceof ApiError)) {
        setProblem({ kind: 'server', isNetwork: false });
      } else if (error.code === 'INVALID_CREDENTIALS') {
        form.setServerErrors({ current: 'That is not your current password.' });
        document.getElementById('security-current')?.focus();
      } else if (error.code === 'PASSWORD_NOT_SET') {
        setPhase('google');
      } else if (error.code === 'UNAUTHENTICATED') {
        // The session could not be renewed. The session gate is already
        // sending the person to the login page.
      } else if (error.code === 'ACCOUNT_DEACTIVATED') {
        recheck();
      } else if (error.code === 'VALIDATION_FAILED') {
        const messages: Partial<Record<keyof Values, string>> = {};
        for (const issue of error.issues) {
          if (issue.field === 'currentPassword') {
            messages.current = serverFieldMessage('currentPassword');
          } else if (issue.field === 'newPassword') {
            messages.password = serverFieldMessage('newPassword');
          }
        }
        if (Object.keys(messages).length > 0) form.setServerErrors(messages);
        else setProblem({ kind: 'server', isNetwork: false });
      } else if (error.code === 'RATE_LIMITED') {
        setProblem({ kind: 'rate-limited', seconds: error.retryAfter ?? 60 });
      } else {
        setProblem({
          kind: 'server',
          isNetwork: error.code === 'NETWORK',
          requestId: error.code === 'INTERNAL' ? error.requestId : undefined,
        });
      }
    }
  }

  if (phase === 'google') return <GoogleOnly />;

  if (phase === 'saved') {
    return (
      <div className="flex max-w-xl flex-col items-start gap-5">
        <span
          aria-hidden="true"
          className="inline-flex size-11 items-center justify-center rounded-full border border-success-border bg-success-tint text-success"
        >
          <Check size={24} strokeWidth={1.5} />
        </span>
        <div className="flex flex-col gap-2">
          <h2 className="font-display text-h2">Password changed</h2>
          <p className="text-body text-text-muted">
            You have been signed out on every device, this one included. Log in
            with your new password.
          </p>
        </div>
        <RouteLine state="complete" className="w-full" />
        <ButtonLink href="/login" size="lg">
          Go to log in
        </ButtonLink>
        <p role="status" className="tabular text-caption text-text-faint">
          Redirecting automatically in {redirectIn} seconds.
        </p>
      </div>
    );
  }

  return (
    <form noValidate onSubmit={submit} className="flex max-w-xl flex-col gap-5">
      {problem?.kind === 'rate-limited' && (
        <RateLimitedAlert alertRef={alertRef} remaining={waiting} />
      )}
      {problem?.kind === 'server' && (
        <ServerFaultAlert
          alertRef={alertRef}
          isNetwork={problem.isNetwork}
          requestId={problem.requestId}
        />
      )}
      <PasswordField
        id="security-current"
        label="Current password"
        autoComplete="current-password"
        isRequired
        value={form.values.current}
        readOnly={isLoading}
        error={form.errors.current}
        onChange={(event) => form.setValue('current', event.target.value)}
        onBlur={() => form.blur('current')}
      />
      <PasswordField
        id="security-password"
        label="New password"
        autoComplete="new-password"
        isRequired
        showChecklist
        value={form.values.password}
        readOnly={isLoading}
        error={form.errors.password}
        onChange={(event) => form.setValue('password', event.target.value)}
        onBlur={() => form.blur('password')}
      />
      <PasswordField
        id="security-confirm"
        label="Confirm new password"
        autoComplete="new-password"
        isRequired
        value={form.values.confirm}
        readOnly={isLoading}
        error={form.errors.confirm}
        onChange={(event) => form.setValue('confirm', event.target.value)}
        onBlur={() => form.blur('confirm')}
      />
      <div className="flex flex-col items-start gap-3">
        <Button type="submit" isLoading={isLoading} isDisabled={isWaiting}>
          {isLoading ? 'Saving' : 'Change password'}
        </Button>
        <p className="text-caption text-text-faint">
          Saving signs you out of every device, this one included. Forgot your
          current password? Log out and use “Forgot password?” on the login
          page.
        </p>
      </div>
    </form>
  );
}
