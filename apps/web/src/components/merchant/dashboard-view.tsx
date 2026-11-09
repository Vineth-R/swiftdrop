'use client';

import Link from 'next/link';
import { ButtonLink } from '../ui/button';
import { PageHeader } from '../layout/page-header';
import { Section } from '../layout/section';
import { ApprovalNotice, LoadState, MerchantIdentity, useMerchant } from './shared';

export function MerchantDashboardView() {
  const state = useMerchant();
  return <><PageHeader title="Merchant dashboard" description="Manage your business integration and delivery orders." actions={<ButtonLink href="/merchant/orders">View orders</ButtonLink>} /><LoadState {...state} />{!state.loading && !state.error && state.merchant ? <><ApprovalNotice merchant={state.merchant} /><MerchantIdentity merchant={state.merchant} /><Section title="Get started" description="Keep your integration ready for the next order."><div className="grid gap-4 md:grid-cols-3"><Link href="/merchant/orders" className="rounded-card border border-line bg-surface p-5 transition-colors hover:border-interactive"><span className="text-label">Orders</span><p className="mt-2 text-body-sm text-text-muted">Review delivery status and history.</p></Link><Link href="/merchant/api-keys" className="rounded-card border border-line bg-surface p-5 transition-colors hover:border-interactive"><span className="text-label">API keys</span><p className="mt-2 text-body-sm text-text-muted">Connect your store securely.</p></Link><Link href="/merchant/profile" className="rounded-card border border-line bg-surface p-5 transition-colors hover:border-interactive"><span className="text-label">Business profile</span><p className="mt-2 text-body-sm text-text-muted">Update pickup and contact details.</p></Link></div></Section></> : null}</>;
}
