'use client';

import Link from 'next/link';
import { useEffect, useState, type FormEvent } from 'react';
import { ApiError } from '@/lib/api';
import { registerMerchant } from '@/lib/merchant-api';
import { getZones, type Zone } from '@/lib/pricing-api';
import { Alert } from '../ui/alert';
import { Button, ButtonLink } from '../ui/button';
import { TextField } from '../ui/field';
import { Select } from '../ui/select';
import { merchantError } from './shared';

export function MerchantRegisterForm() {
  const [zones, setZones] = useState<Zone[]>([]);
  const [zonesLoading, setZonesLoading] = useState(true);
  const [zonesError, setZonesError] = useState(false);
  const [revision, setRevision] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState(false);
  useEffect(() => {
    const controller = new AbortController(); setZonesLoading(true); setZonesError(false);
    getZones(controller.signal).then((data) => { if (!controller.signal.aborted) setZones(data); }).catch(() => { if (!controller.signal.aborted) setZonesError(true); }).finally(() => { if (!controller.signal.aborted) setZonesLoading(false); });
    return () => controller.abort();
  }, [revision]);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (busy || zonesLoading || zonesError) return;
    const form = event.currentTarget;
    const data = new FormData(form);
    const value = (name: string) => String(data.get(name) || '').trim();
    const password = String(data.get('password') || '');
    if (password.length < 8 || !/[A-Z]/.test(password) || !/[a-z]/.test(password) || !/\d/.test(password)) { setError('Use at least 8 characters, including an uppercase letter, a lowercase letter and a number.'); return; }
    setBusy(true); setError('');
    try {
      await registerMerchant({ fullName: value('fullName'), email: value('email'), phone: value('phone'), password, business: { name: value('businessName'), registrationNumber: value('registrationNumber') || undefined, contactEmail: value('contactEmail'), contactPhone: value('contactPhone'), pickupAddress: value('pickupAddress'), pickupZoneId: value('pickupZoneId') } });
      form.reset(); setSuccess(true);
    } catch (err) { setError(err instanceof ApiError && err.code === 'EMAIL_IN_USE' ? 'An account already uses this email. Sign in or use a different email.' : merchantError(err)); }
    finally { setBusy(false); }
  }
  if (success) return <div className="flex flex-col gap-5"><Alert tone="success" title="Your registration is awaiting approval">Your business has been registered. Sign in to check your approval status and manage your business profile.</Alert><ButtonLink href="/login">Continue to sign in</ButtonLink></div>;
  return <form onSubmit={submit} className="flex flex-col gap-6">
    {error ? <Alert tone="danger" title="Could not register your business">{error}</Alert> : null}
    <fieldset disabled={busy} className="flex flex-col gap-4"><legend className="mb-4 font-display text-h3">Your account</legend>
      <TextField id="merchant-full-name" name="fullName" label="Full name" required maxLength={150} autoComplete="name" />
      <div className="grid gap-4 sm:grid-cols-2"><TextField id="merchant-email" name="email" label="Email" type="email" required maxLength={255} autoComplete="email" /><TextField id="merchant-phone" name="phone" label="Phone number" type="tel" required maxLength={20} autoComplete="tel" /></div>
      <TextField id="merchant-password" name="password" label="Password" type="password" required minLength={8} autoComplete="new-password" helper="At least 8 characters, with uppercase, lowercase and a number." />
    </fieldset>
    <fieldset disabled={busy} className="flex flex-col gap-4 border-t border-line pt-6"><legend className="font-display text-h3">Your business</legend>
      <TextField id="merchant-business-name" name="businessName" label="Business name" required maxLength={150} autoComplete="organization" />
      <TextField id="merchant-registration-number" name="registrationNumber" label="Registration number (optional)" maxLength={50} />
      <div className="grid gap-4 sm:grid-cols-2"><TextField id="merchant-contact-email" name="contactEmail" label="Business email" type="email" required maxLength={255} /><TextField id="merchant-contact-phone" name="contactPhone" label="Business phone" type="tel" required maxLength={20} /></div>
      <TextField id="merchant-pickup-address" name="pickupAddress" label="Pickup address" required maxLength={255} autoComplete="street-address" />
      <Select id="merchant-pickup-zone" name="pickupZoneId" label="Pickup zone" required defaultValue="" placeholder={zonesLoading ? 'Loading zones…' : 'Choose your pickup zone'} options={zones.map((zone) => ({ value: zone.id, label: zone.name }))} disabled={zonesLoading || zonesError} />
    </fieldset>
    {zonesError ? <Alert tone="danger" title="Could not load pickup zones" actions={<Button variant="secondary" onClick={() => setRevision((n) => n + 1)}>Retry zones</Button>}>Please retry before registering.</Alert> : !zonesLoading && !zones.length ? <Alert tone="warning" title="No pickup zones available">Please try again later.</Alert> : null}
    <p className="text-caption text-text-muted">An administrator reviews each business before live orders and API keys are enabled.</p>
    <Button type="submit" isFullWidth isLoading={busy} isDisabled={zonesLoading || zonesError || !zones.length}>{busy ? 'Registering…' : 'Register business'}</Button>
    <p className="text-center text-body-sm text-text-muted">Already registered? <Link href="/login" className="text-interactive underline underline-offset-4">Log in</Link></p>
  </form>;
}
