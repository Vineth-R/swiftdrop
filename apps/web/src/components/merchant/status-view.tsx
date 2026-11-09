'use client';
import { PageHeader } from '../layout/page-header';
import { Alert } from '../ui/alert';
import { Button, ButtonLink } from '../ui/button';
import { ApprovalNotice, LoadState, MerchantIdentity, useMerchant } from './shared';

export function MerchantStatusView() {
  const state = useMerchant();
  return <><PageHeader title="Merchant approval" description="Check your business registration status." actions={<Button variant="secondary" isDisabled={state.loading} onClick={state.retry}>Check status</Button>} /><LoadState {...state} />{!state.loading && !state.error && state.merchant ? <><MerchantIdentity merchant={state.merchant} /><ApprovalNotice merchant={state.merchant} />{state.merchant.status === 'APPROVED' ? <Alert tone="success" title="Your business is approved">You can create API keys and accept delivery orders.</Alert> : null}<div className="flex flex-wrap gap-3"><ButtonLink href="/merchant/profile" variant="secondary">Business profile</ButtonLink>{state.merchant.status === 'APPROVED' ? <ButtonLink href="/merchant/api-keys">Manage API keys</ButtonLink> : null}</div></> : null}</>;
}
