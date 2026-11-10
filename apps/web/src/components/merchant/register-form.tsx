'use client';

import Link from 'next/link';
import { useEffect, useState, type FormEvent } from 'react';

import { ApiError } from '@/lib/api';
import { registerMerchant } from '@/lib/merchant-api';
import { getZones, type Zone } from '@/lib/pricing-api';
import { Alert } from '../ui/alert';
import { Button, ButtonLink } from '../ui/button';
import { TextField } from '../ui/field';
import { PasswordField } from '../ui/password-field';
import { Select } from '../ui/select';
import { merchantError } from './shared';

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

export function MerchantRegisterForm() {
  const [values, setValues] = useState<Values>(empty);
  const [zones, setZones] = useState<Zone[]>([]);
  const [zonesLoading, setZonesLoading] = useState(true);
  const [zonesError, setZonesError] = useState(false);
  const [revision, setRevision] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [fieldError, setFieldError] = useState('');
  const [success, setSuccess] = useState<{ merchantId: string } | null>(null);
  const [idempotencyKey] = useState(() => crypto.randomUUID());

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

  const set = (key: keyof Values) => (event: { target: { value: string } }) =>
    setValues((old) => ({ ...old, [key]: event.target.value }));

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy || zonesLoading || zonesError) return;
    setError('');
    setFieldError('');
    if (values.password !== values.confirm) {
      setFieldError('The passwords do not match.');
      return;
    }
    setBusy(true);
    try {
      const result = await registerMerchant(
        {
          fullName: values.fullName,
          email: values.email,
          phone: values.phone,
          password: values.password,
          business: {
            name: values.businessName,
            registrationNumber: values.registrationNumber,
            contactEmail: values.contactEmail || values.email,
            contactPhone: values.contactPhone || values.phone,
            businessAddress: values.businessAddress,
            pickupAddress: values.pickupAddress,
            pickupZoneId: values.pickupZoneId,
          },
        },
        idempotencyKey,
      );
      setSuccess({ merchantId: result.merchantId });
    } catch (err) {
      setError(
        err instanceof ApiError && err.code === 'EMAIL_IN_USE'
          ? 'An account already uses this email. Sign in or use a different email.'
          : err instanceof ApiError &&
              err.code === 'DUPLICATE_MERCHANT_APPLICATION'
            ? 'A merchant application already exists for this email or business registration number.'
            : merchantError(err),
      );
    } finally {
      setBusy(false);
    }
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

  return (
    <form onSubmit={submit} className="flex flex-col gap-6">
      {error ? (
        <Alert tone="danger" title="Could not register your business">
          {error}
        </Alert>
      ) : null}
      {fieldError ? (
        <Alert tone="danger" title="Check your details">
          {fieldError}
        </Alert>
      ) : null}

      <fieldset disabled={busy} className="flex flex-col gap-4">
        <legend className="mb-2 font-display text-h3">Your account</legend>
        <TextField
          id="merchant-full-name"
          label="Full name"
          required
          maxLength={150}
          autoComplete="name"
          value={values.fullName}
          onChange={set('fullName')}
        />
        <div className="grid gap-4 sm:grid-cols-2">
          <TextField
            id="merchant-email"
            label="Email"
            type="email"
            required
            maxLength={255}
            autoComplete="email"
            value={values.email}
            onChange={set('email')}
          />
          <TextField
            id="merchant-phone"
            label="Phone number"
            type="tel"
            required
            maxLength={20}
            autoComplete="tel"
            helper="E.164 format, for example +94771234567"
            value={values.phone}
            onChange={set('phone')}
          />
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <PasswordField
            id="merchant-password"
            label="Password"
            required
            autoComplete="new-password"
            showChecklist
            value={values.password}
            onChange={set('password')}
          />
          <PasswordField
            id="merchant-confirm"
            label="Confirm password"
            required
            autoComplete="new-password"
            value={values.confirm}
            onChange={set('confirm')}
          />
        </div>
      </fieldset>

      <fieldset
        disabled={busy}
        className="flex flex-col gap-4 border-t border-line pt-6"
      >
        <legend className="mb-2 font-display text-h3">Your business</legend>
        <TextField
          id="merchant-business-name"
          label="Business name"
          required
          maxLength={150}
          autoComplete="organization"
          value={values.businessName}
          onChange={set('businessName')}
        />
        <TextField
          id="merchant-registration-number"
          label="Business registration number"
          required
          maxLength={50}
          helper="For example PV 123456. Letters, digits, spaces, hyphens, slashes and dots are accepted."
          value={values.registrationNumber}
          onChange={set('registrationNumber')}
        />
        <div className="grid gap-4 sm:grid-cols-2">
          <TextField
            id="merchant-contact-email"
            label="Business email"
            type="email"
            maxLength={255}
            helper="Defaults to your account email."
            value={values.contactEmail}
            onChange={set('contactEmail')}
          />
          <TextField
            id="merchant-contact-phone"
            label="Business phone"
            type="tel"
            maxLength={20}
            helper="Defaults to your account phone."
            value={values.contactPhone}
            onChange={set('contactPhone')}
          />
        </div>
        <TextField
          id="merchant-business-address"
          label="Business address"
          required
          maxLength={255}
          autoComplete="street-address"
          value={values.businessAddress}
          onChange={set('businessAddress')}
        />
        <TextField
          id="merchant-pickup-address"
          label="Default pickup address"
          required
          maxLength={255}
          autoComplete="street-address"
          value={values.pickupAddress}
          onChange={set('pickupAddress')}
        />
        <Select
          id="merchant-pickup-zone"
          label="Default pickup zone"
          required
          value={values.pickupZoneId}
          placeholder={
            zonesLoading ? 'Loading zones…' : 'Choose your pickup zone'
          }
          options={zones.map((zone) => ({ value: zone.id, label: zone.name }))}
          disabled={zonesLoading || zonesError}
          onChange={set('pickupZoneId')}
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
        isFullWidth
        isLoading={busy}
        isDisabled={zonesLoading || zonesError || !zones.length}
      >
        {busy ? 'Submitting application…' : 'Submit application'}
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
  );
}
