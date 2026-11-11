'use client';

import { Ban, Lock } from 'lucide-react';
import Link from 'next/link';
import { useRouter, useSearchParams } from 'next/navigation';
import {
  useEffect,
  useRef,
  useState,
  type FocusEvent,
  type FormEvent,
} from 'react';

import { ApiError } from '@/lib/api';
import { classifyApiError, type ApiProblem } from '@/lib/api-problem';
import { googleLoginUrl, resendVerification } from '@/lib/auth-api';
import { DEFAULT_AUTH_ROUTE } from '@/lib/constants';
import { homeFor } from '@/lib/nav';
import { safeNext } from '@/lib/redirect';
import { useSession } from '@/lib/session';
import { formatClock, useCountdown } from '@/lib/use-countdown';
import { useForm, type FormRules } from '@/lib/use-form';
import {
  serverFieldMessage,
  validateEmail,
  validateRequiredPassword,
} from '@/lib/validation';
import { Alert } from '@/components/ui/alert';
import { Button, ButtonLink } from '@/components/ui/button';
import { TextField } from '@/components/ui/field';
import { PasswordField } from '@/components/ui/password-field';
import { ApiProblemAlerts } from './api-problem-alerts';
import { GoogleButton, OrDivider } from './google-button';

type Banner =
  | { kind: 'invalid' }
  | { kind: 'locked' }
  | { kind: 'oauth' }
  | { kind: 'verification' }
  | { kind: 'merchant-inactive' }
  | { kind: 'verified' }
  | ApiProblem;

const rules: FormRules<{ email: string; password: string }> = {
  email: (v) => validateEmail(v.email),
  password: (v) => validateRequiredPassword(v.password),
};

export function LoginForm() {
  const router = useRouter();
  const params = useSearchParams();
  const { signIn } = useSession();
  const next = safeNext(params.get('next'), '');

  const form = useForm({ email: '', password: '' }, rules);
  const [banner, setBanner] = useState<Banner | null>(
    params.get('error') === 'oauth_failed'
      ? { kind: 'oauth' }
      : params.get('verified') === '1'
        ? { kind: 'verified' }
        : null,
  );
  const [resent, setResent] = useState(false);
  const [isDeactivated, setIsDeactivated] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const alertRef = useRef<HTMLDivElement>(null);

  const waitSeconds = banner?.kind === 'rate-limited' ? banner.seconds : null;
  const remaining = useCountdown(waitSeconds);
  const isWaiting = banner?.kind === 'rate-limited' && remaining > 0;

  useEffect(() => {
    if (banner) alertRef.current?.focus();
  }, [banner]);

  /** Leaving a field counts as touching it, unless focus only moved to the alert. */
  function leave(
    event: FocusEvent<HTMLInputElement>,
    field: 'email' | 'password',
  ) {
    const movedToAlert =
      alertRef.current !== null && event.relatedTarget === alertRef.current;
    if (!movedToAlert) form.blur(field);
  }

  async function resend() {
    try {
      await resendVerification(form.values.email);
      setResent(true);
    } catch {
      setResent(false);
    }
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isLoading || isWaiting) return;
    setBanner(null);

    const found = form.validate();
    if (found.email || found.password) {
      document
        .getElementById(found.email ? 'login-email' : 'login-password')
        ?.focus();
      return;
    }

    setIsLoading(true);
    try {
      const user = await signIn(form.values.email, form.values.password);
      router.replace(next || homeFor(user.role));
    } catch (error) {
      setIsLoading(false);
      handleFailure(error);
    }
  }

  function handleFailure(error: unknown) {
    if (error instanceof ApiError) {
      switch (error.code) {
        case 'INVALID_CREDENTIALS':
          // Clear the password quietly: the alert already says what is wrong.
          form.setValue('password', '');
          form.resetValidation();
          setBanner({ kind: 'invalid' });
          return;
        case 'ACCOUNT_LOCKED':
          setBanner({ kind: 'locked' });
          return;
        case 'ACCOUNT_DEACTIVATED':
          setIsDeactivated(true);
          return;
        case 'EMAIL_VERIFICATION_REQUIRED':
          setBanner({ kind: 'verification' });
          return;
        case 'MERCHANT_ACCOUNT_INACTIVE':
          setBanner({ kind: 'merchant-inactive' });
          return;
      }
    }

    const classified = classifyApiError(error);
    if (classified.kind === 'validation') {
      const fields: Partial<Record<'email' | 'password', string>> = {};
      for (const issue of classified.issues) {
        const message = serverFieldMessage(issue.field);
        if (issue.field === 'email' || issue.field === 'password') {
          fields[issue.field] = message;
        }
      }
      if (fields.email || fields.password) {
        form.setServerErrors(fields);
        return;
      }
    }
    setBanner(classified);
  }

  if (isDeactivated) {
    return (
      <div className="flex flex-col gap-4">
        <Alert
          ref={alertRef}
          tone="danger"
          icon={Ban}
          title="This account has been deactivated"
        >
          An administrator turned it off. Contact support if you think this is a
          mistake.
        </Alert>
        <ButtonLink href="/" variant="secondary" size="lg" isFullWidth>
          Back to home
        </ButtonLink>
      </div>
    );
  }

  return (
    <>
      {banner && (
        <LoginBanner
          banner={banner}
          alertRef={alertRef}
          remaining={remaining}
          email={form.values.email}
          resent={resent}
          onResend={resend}
        />
      )}

      <form noValidate onSubmit={submit} className="flex flex-col gap-5">
        <TextField
          id="login-email"
          label="Email"
          type="email"
          autoComplete="email"
          placeholder="you@example.lk"
          controlSize="lg"
          value={form.values.email}
          readOnly={isLoading}
          error={form.errors.email}
          onChange={(event) => form.setValue('email', event.target.value)}
          onBlur={(event) => leave(event, 'email')}
        />
        <PasswordField
          id="login-password"
          label="Password"
          autoComplete="current-password"
          controlSize="lg"
          value={form.values.password}
          readOnly={isLoading}
          error={form.errors.password}
          onChange={(event) => form.setValue('password', event.target.value)}
          onBlur={(event) => leave(event, 'password')}
          labelAction={
            <Link
              href="/forgot-password"
              className="text-body-sm text-interactive underline underline-offset-4 hover:text-interactive-hover"
            >
              Forgot password?
            </Link>
          }
        />
        <Button
          type="submit"
          size="lg"
          isFullWidth
          isLoading={isLoading}
          isDisabled={isWaiting}
          className="tabular"
        >
          {isLoading
            ? 'Logging in'
            : isWaiting
              ? `Try again in ${formatClock(remaining)}`
              : 'Log in'}
        </Button>
      </form>

      <OrDivider />
      <GoogleButton href={googleLoginUrl(next || DEFAULT_AUTH_ROUTE)}>
        Continue with Google
      </GoogleButton>
    </>
  );
}

function LoginBanner({
  banner,
  alertRef,
  remaining,
  email,
  resent,
  onResend,
}: {
  banner: Banner;
  alertRef: React.Ref<HTMLDivElement>;
  remaining: number;
  email: string;
  resent: boolean;
  onResend: () => void;
}) {
  switch (banner.kind) {
    case 'verified':
      return (
        <Alert
          ref={alertRef}
          tone="success"
          title="Your email has been verified"
        >
          You can now sign in to SwiftDrop.
        </Alert>
      );
    case 'verification':
      return (
        <Alert ref={alertRef} tone="warning" title="Verify your email first">
          <p>
            We sent a verification link when you registered. Open it before
            signing in.
          </p>
          <div className="mt-3">
            {resent ? (
              <p>
                If that address needs verification, a new link is on its way.
              </p>
            ) : (
              <Button
                variant="secondary"
                onClick={onResend}
                isDisabled={!email.trim()}
              >
                Send another verification email
              </Button>
            )}
          </div>
        </Alert>
      );
    case 'merchant-inactive':
      return (
        <Alert
          ref={alertRef}
          tone="warning"
          title="Merchant account not active"
        >
          Merchant accounts can sign in only after an administrator approves the
          application. Check the decision email, or contact support if you need
          help.
        </Alert>
      );
    case 'invalid':
      return (
        <Alert
          ref={alertRef}
          tone="danger"
          title="Email or password is incorrect"
        >
          Check both and try again.
        </Alert>
      );
    case 'locked':
      return (
        <Alert
          ref={alertRef}
          tone="danger"
          icon={Lock}
          title="Account locked for 15 minutes"
        >
          Too many failed attempts. Try again later, or{' '}
          <Link
            href="/forgot-password"
            className="font-semibold text-interactive underline underline-offset-4"
          >
            reset your password now
          </Link>
          .
        </Alert>
      );
    case 'oauth':
      return (
        <Alert
          ref={alertRef}
          tone="danger"
          title="Google sign-in did not complete"
        >
          Please try again, or log in with your email and password.
        </Alert>
      );
    default:
      return (
        <ApiProblemAlerts
          problem={banner}
          alertRef={alertRef}
          remaining={remaining}
        />
      );
  }
}
