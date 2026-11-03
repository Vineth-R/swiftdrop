'use client';

import { useEffect, useMemo, useRef, useState, type FormEvent } from 'react';

import { ApiError } from '../../lib/api';
import {
  updateProfile,
  type ProfileChange,
  type User,
} from '../../lib/auth-api';
import { useSession } from '../../lib/session';
import { useCountdown } from '../../lib/use-countdown';
import { useForm, type FormRules } from '../../lib/use-form';
import {
  serverFieldMessage,
  toInternational,
  toSriLankanE164,
  toSriLankanLocal,
  validateFullName,
  validateInternationalPhone,
  validatePhone,
} from '../../lib/validation';
import { RateLimitedAlert, ServerFaultAlert } from '../auth/form-alerts';
import { Alert } from '../ui/alert';
import { Button } from '../ui/button';
import { TextField } from '../ui/field';

type Values = { fullName: string; phone: string };

/**
 * A Sri Lankan number (or none yet, as after a Google sign-up) is typed after a
 * fixed +94. Any other number is shown and edited whole, with its country code.
 */
function isLocalNumber(user: User): boolean {
  return user.phone === '' || toSriLankanLocal(user.phone) !== undefined;
}

function valuesOf(user: User): Values {
  return {
    fullName: user.fullName,
    phone: isLocalNumber(user)
      ? (toSriLankanLocal(user.phone) ?? '')
      : user.phone,
  };
}

/** Only what the person actually changed, so the API never sees untouched fields. */
function changesOf(user: User, values: Values): ProfileChange {
  const change: ProfileChange = {};
  const name = values.fullName.trim();
  if (name !== user.fullName) change.fullName = name;
  if (values.phone.trim() !== '') {
    const phone = isLocalNumber(user)
      ? toSriLankanE164(values.phone)
      : toInternational(values.phone);
    if (phone !== user.phone) change.phone = phone;
  }
  return change;
}

type Problem =
  | { kind: 'rate-limited'; seconds: number }
  | { kind: 'server'; isNetwork: boolean; requestId?: string };

export function ProfileForm({ user }: { user: User }) {
  const { setUser, recheck } = useSession();
  const isLocal = isLocalNumber(user);
  const hadPhone = user.phone !== '';

  // Defined with useMemo, not at module level, because the phone rule depends
  // on this account; useForm only needs the object to stay the same between renders.
  const rules = useMemo<FormRules<Values>>(
    () => ({
      fullName: (v) => validateFullName(v.fullName),
      phone: (v) => {
        if (v.phone.trim() === '') {
          return hadPhone
            ? 'Enter a full number, for example 71 234 5678.'
            : undefined;
        }
        return isLocal
          ? validatePhone(v.phone)
          : validateInternationalPhone(v.phone);
      },
    }),
    [hadPhone, isLocal],
  );

  const form = useForm<Values>(valuesOf(user), rules);
  const [isLoading, setIsLoading] = useState(false);
  const [isSaved, setIsSaved] = useState(false);
  const [problem, setProblem] = useState<Problem | null>(null);
  const alertRef = useRef<HTMLDivElement>(null);

  const waiting = useCountdown(
    problem?.kind === 'rate-limited' ? problem.seconds : null,
  );
  const isWaiting = problem?.kind === 'rate-limited' && waiting > 0;
  const isDirty = Object.keys(changesOf(user, form.values)).length > 0;

  useEffect(() => {
    if (problem) alertRef.current?.focus();
  }, [problem]);

  function edit(key: keyof Values, value: string) {
    setIsSaved(false);
    form.setValue(key, value);
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isLoading || isWaiting) return;
    setProblem(null);
    setIsSaved(false);

    const found = form.validate();
    if (found.fullName || found.phone) {
      document
        .getElementById(found.fullName ? 'profile-name' : 'profile-phone')
        ?.focus();
      return;
    }
    const change = changesOf(user, form.values);
    if (Object.keys(change).length === 0) return;

    setIsLoading(true);
    try {
      const updated = await updateProfile(change);
      setUser(updated);
      const next = valuesOf(updated);
      form.setValue('fullName', next.fullName);
      form.setValue('phone', next.phone);
      form.resetValidation();
      setIsSaved(true);
    } catch (error) {
      if (!(error instanceof ApiError)) {
        setProblem({ kind: 'server', isNetwork: false });
      } else if (error.code === 'UNAUTHENTICATED') {
        // The session could not be renewed. The session gate is already
        // sending the person to the login page.
      } else if (error.code === 'ACCOUNT_DEACTIVATED') {
        recheck();
      } else if (error.code === 'VALIDATION_FAILED') {
        const messages: Partial<Record<keyof Values, string>> = {};
        for (const issue of error.issues) {
          if (issue.field === 'fullName' || issue.field === 'phone') {
            messages[issue.field] = serverFieldMessage(issue.field);
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
    } finally {
      setIsLoading(false);
    }
  }

  return (
    <form noValidate onSubmit={submit} className="flex max-w-xl flex-col gap-5">
      {isSaved && (
        <Alert tone="success" title="Details saved">
          Your new details are on your account.
        </Alert>
      )}
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
      <TextField
        id="profile-name"
        label="Full name"
        autoComplete="name"
        isRequired
        helper="This is the name couriers and merchants see."
        value={form.values.fullName}
        readOnly={isLoading}
        error={form.errors.fullName}
        onChange={(event) => edit('fullName', event.target.value)}
        onBlur={() => form.blur('fullName')}
      />
      <TextField
        id="profile-email"
        label="Email"
        type="email"
        readOnly
        value={user.email}
        helper="Your email is how you log in, so it cannot be changed here."
      />
      <TextField
        id="profile-phone"
        label="Phone"
        type="tel"
        autoComplete={isLocal ? 'tel-national' : 'tel'}
        inputMode="tel"
        isRequired={hadPhone}
        isMono
        prefix={isLocal ? '+94' : undefined}
        helper={
          isLocal
            ? 'Used for delivery calls.'
            : 'Include the country code, for example +44 20 7123 4567.'
        }
        value={form.values.phone}
        readOnly={isLoading}
        error={form.errors.phone}
        onChange={(event) => edit('phone', event.target.value)}
        onBlur={() => form.blur('phone')}
      />
      <div className="flex items-center gap-3">
        <Button
          type="submit"
          isLoading={isLoading}
          isDisabled={!isDirty || isWaiting}
        >
          {isLoading ? 'Saving' : 'Save changes'}
        </Button>
        {!isDirty && !isSaved && (
          <span className="text-caption text-text-faint">
            Change a field to save.
          </span>
        )}
      </div>
    </form>
  );
}
