'use client';

import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { useEffect, useRef, useState, type FormEvent } from 'react';

import { ApiError } from '@/lib/api';
import { resendVerification, verifyEmail } from '@/lib/auth-api';
import { Alert } from '../ui/alert';
import { Button, ButtonLink } from '../ui/button';
import { TextField } from '../ui/field';

type State =
  | { kind: 'loading' }
  | { kind: 'success' }
  | { kind: 'invalid' }
  | { kind: 'unavailable' };

export function VerifyEmailView() {
  const params = useSearchParams();
  const token = params.get('token') ?? '';
  const [state, setState] = useState<State>({ kind: 'loading' });
  const [email, setEmail] = useState('');
  const [resent, setResent] = useState(false);
  const [resendError, setResendError] = useState('');
  const [busy, setBusy] = useState(false);
  const started = useRef(false);

  useEffect(() => {
    if (started.current) return;
    started.current = true;
    if (!token) {
      setState({ kind: 'invalid' });
    } else {
      // Never keep the single-use token in the address bar or history.
      window.history.replaceState(null, '', '/verify-email');
      verifyEmail(token)
        .then(() => setState({ kind: 'success' }))
        .catch((error: unknown) => {
          setState(
            error instanceof ApiError &&
              error.code === 'INVALID_VERIFICATION_TOKEN'
              ? { kind: 'invalid' }
              : { kind: 'unavailable' },
          );
        });
    }
  }, [token]);

  async function resend(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy || !email.trim()) return;
    setBusy(true);
    setResendError('');
    try {
      await resendVerification(email);
      setResent(true);
    } catch (error) {
      setResendError(
        error instanceof ApiError && error.code === 'RATE_LIMITED'
          ? 'Too many requests. Wait a minute and try again.'
          : 'We could not request another email. Check your connection and try again.',
      );
    } finally {
      setBusy(false);
    }
  }

  if (state.kind === 'loading') {
    return (
      <p role="status" className="text-body text-text-muted">
        Verifying your email…
      </p>
    );
  }

  if (state.kind === 'success') {
    return (
      <div className="flex flex-col gap-4">
        <Alert tone="success" title="Your email has been verified">
          You can now sign in to SwiftDrop.
        </Alert>
        <ButtonLink href="/login?verified=1" size="lg" isFullWidth>
          Go to log in
        </ButtonLink>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <Alert
        tone="danger"
        title={
          state.kind === 'invalid'
            ? 'This link is invalid or has expired'
            : 'We could not verify your email'
        }
      >
        {state.kind === 'invalid'
          ? 'Request another verification email below. Do not create a new account.'
          : 'Check your connection and try again, or request another verification email.'}
      </Alert>
      {resent ? (
        <Alert tone="success" title="Check your email">
          If that address needs verification, a new link is on its way. It
          expires in 30 minutes.
        </Alert>
      ) : (
        <form onSubmit={resend} className="flex flex-col gap-4">
          {resendError ? (
            <Alert tone="danger" title="Could not send another email">
              {resendError}
            </Alert>
          ) : null}
          <TextField
            id="verify-email-address"
            label="Email"
            type="email"
            autoComplete="email"
            required
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            helper="Use the email address you registered with."
          />
          <Button type="submit" size="lg" isFullWidth isLoading={busy}>
            {busy ? 'Requesting…' : 'Send another verification email'}
          </Button>
        </form>
      )}
      <p className="text-center text-body-sm text-text-muted">
        <Link
          href="/login"
          className="text-interactive underline underline-offset-4"
        >
          Back to log in
        </Link>
      </p>
    </div>
  );
}
