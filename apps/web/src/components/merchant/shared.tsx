'use client';

import { useEffect, useState } from 'react';
import { ApiError } from '@/lib/api';
import { getMerchant, type Merchant } from '@/lib/merchant-api';
import { Alert } from '../ui/alert';
import { Button } from '../ui/button';
import { StatusChip } from '../ui/status-chip';

export function merchantError(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 404 || error.status === 503) return 'This merchant service is currently unavailable. Please try again later.';
    if (error.status === 401) return 'Your merchant session could not be verified. Please sign in again.';
    if (error.status === 403) return 'Your account does not have access to this action. Check your approval status.';
    if (error.status === 409) return 'This action conflicts with an existing record. Refresh and try again.';
    if (error.status === 422) return 'Check your details and try again.';
    if (error.status === 429) return 'Too many attempts. Please wait before trying again.';
  }
  return 'We could not complete this request. Check your connection and try again.';
}

export function useMerchant() {
  const [merchant, setMerchant] = useState<Merchant | null>(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true); setError('');
    getMerchant(controller.signal).then((data) => {
      if (!controller.signal.aborted) setMerchant(data);
    }).catch((err) => {
      if (!controller.signal.aborted) setError(merchantError(err));
    }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [revision]);
  return { merchant, setMerchant, error, loading, retry: () => setRevision((n) => n + 1) };
}

export function LoadState({ loading, error, retry }: { loading: boolean; error: string; retry: () => void }) {
  if (loading) return <p role="status" className="text-body text-text-muted">Loading merchant details…</p>;
  if (error) return <Alert tone="danger" title="Could not load merchant details" actions={<Button variant="secondary" onClick={retry}>Try again</Button>}>{error}</Alert>;
  return null;
}

export function ApprovalNotice({ merchant }: { merchant: Merchant }) {
  if (merchant.status === 'APPROVED') return null;
  const copy = {
    PENDING: ['Merchant account pending approval', 'Your merchant registration is awaiting administrator approval. You can manage your business profile while you wait.'],
    REJECTED: ['Merchant registration rejected', merchant.rejectionReason || 'Contact support to discuss your registration.'],
    SUSPENDED: ['Merchant API access suspended', merchant.suspensionReason || 'Your API keys cannot accept orders while your account is suspended.'],
  } as const;
  const [title, description] = copy[merchant.status];
  return <Alert tone="warning" title={title}>{description}</Alert>;
}

export function MerchantIdentity({ merchant }: { merchant: Merchant }) {
  return <div className="flex flex-wrap items-center justify-between gap-4 rounded-card border border-line bg-surface p-6"><div><p className="text-overline uppercase text-text-muted">Business account</p><h2 className="mt-1 font-display text-h3">{merchant.businessName}</h2><p className="mt-1 text-body-sm text-text-muted">{merchant.registrationNumber || 'No registration number provided'}</p></div><StatusChip status={merchant.status} /></div>;
}

export function displayDate(value?: string | null) {
  if (!value) return 'Never';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '—' : new Intl.DateTimeFormat('en-GB', { dateStyle: 'medium' }).format(date);
}
