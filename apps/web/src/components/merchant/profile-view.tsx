'use client';

import { useEffect, useState, type FormEvent } from 'react';
import { getZones, type Zone } from '@/lib/pricing-api';
import { updateMerchant, type Merchant, type MerchantProfileInput } from '@/lib/merchant-api';
import { PageHeader } from '../layout/page-header';
import { Section } from '../layout/section';
import { Alert } from '../ui/alert';
import { Button } from '../ui/button';
import { TextField } from '../ui/field';
import { Select } from '../ui/select';
import { ApprovalNotice, LoadState, MerchantIdentity, merchantError, useMerchant } from './shared';

export function BusinessProfileView() {
  const state = useMerchant();
  return <><PageHeader title="Business profile" description="Your business details and default parcel pickup location." /><LoadState {...state} />{!state.loading && !state.error && state.merchant ? <><ApprovalNotice merchant={state.merchant} /><MerchantIdentity merchant={state.merchant} /><BusinessForm key={state.merchant.id} merchant={state.merchant} onSaved={state.setMerchant} /></> : null}</>;
}

function BusinessForm({ merchant, onSaved }: { merchant: Merchant; onSaved: (merchant: Merchant) => void }) {
  const [values, setValues] = useState<MerchantProfileInput>({ contactEmail: merchant.contactEmail, contactPhone: merchant.contactPhone, pickupAddress: merchant.pickupAddress, pickupZoneId: merchant.pickupZoneId });
  const [zones, setZones] = useState<Zone[]>([]);
  const [zoneError, setZoneError] = useState('');
  const [zoneRevision, setZoneRevision] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [saved, setSaved] = useState(false);
  useEffect(() => {
    const controller = new AbortController(); setZoneError('');
    getZones(controller.signal).then((data) => { if (!controller.signal.aborted) setZones(data); }).catch(() => { if (!controller.signal.aborted) setZoneError('Pickup zones could not be loaded. Your existing zone will be kept.'); });
    return () => controller.abort();
  }, [zoneRevision]);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (busy) return;
    setBusy(true); setError(''); setSaved(false);
    try { const result = await updateMerchant(values); onSaved(result); setSaved(true); } catch (err) { setError(merchantError(err)); } finally { setBusy(false); }
  }
  const change = (key: keyof MerchantProfileInput, value: string) => { setValues((old) => ({ ...old, [key]: value })); setSaved(false); };
  const options = zones.map((zone) => ({ value: zone.id, label: zone.name }));
  if (!options.some((zone) => zone.value === values.pickupZoneId)) options.unshift({ value: values.pickupZoneId, label: 'Current pickup zone' });
  return <Section title="Contact and pickup" description="These pickup details are used when an API order does not specify a pickup location."><form onSubmit={submit} className="flex max-w-3xl flex-col gap-6 rounded-card border border-line bg-surface p-5 md:p-6">
    {error ? <Alert tone="danger" title="Could not save changes">{error}</Alert> : null}{saved ? <Alert tone="success" title="Business profile updated" /> : null}
    <fieldset disabled={busy} className="grid gap-5 md:grid-cols-2"><legend className="sr-only">Business contact and pickup details</legend>
      <TextField id="business-contact-email" label="Contact email" type="email" required maxLength={255} value={values.contactEmail} onChange={(e) => change('contactEmail', e.target.value)} autoComplete="email" />
      <TextField id="business-contact-phone" label="Contact phone" type="tel" required maxLength={20} value={values.contactPhone} onChange={(e) => change('contactPhone', e.target.value)} autoComplete="tel" />
      <TextField id="business-pickup-address" label="Pickup address" required maxLength={255} value={values.pickupAddress} onChange={(e) => change('pickupAddress', e.target.value)} autoComplete="street-address" />
      <Select id="business-pickup-zone" label="Pickup zone" required value={values.pickupZoneId} options={options} onChange={(e) => change('pickupZoneId', e.target.value)} />
    </fieldset>
    {zoneError ? <Alert tone="warning" title="Pickup zones unavailable" actions={<Button variant="secondary" onClick={() => setZoneRevision((n) => n + 1)}>Retry zones</Button>}>{zoneError}</Alert> : null}
    <div className="flex flex-wrap items-center gap-4"><Button type="submit" isLoading={busy}>{busy ? 'Saving…' : 'Save changes'}</Button><p className="text-caption text-text-muted">Business name and registration number cannot be changed here.</p></div>
  </form></Section>;
}
