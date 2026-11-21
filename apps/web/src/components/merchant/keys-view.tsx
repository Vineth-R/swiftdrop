'use client';

import { Copy, KeyRound, Plus } from 'lucide-react';
import { useEffect, useState, type FormEvent } from 'react';
import { createMerchantKey, listMerchantKeys, revokeMerchantKey, type CreatedMerchantKey, type MerchantKey } from '@/lib/merchant-api';
import { PageHeader } from '../layout/page-header';
import { Section } from '../layout/section';
import { Alert } from '../ui/alert';
import { Button } from '../ui/button';
import { EmptyState } from '../ui/empty-state';
import { TextField } from '../ui/field';
import { Modal } from '../ui/modal';
import { ApprovalNotice, LoadState, displayDate, merchantError, useMerchant } from './shared';

export function MerchantKeysView() {
  const profile = useMerchant();
  const [keys, setKeys] = useState<MerchantKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [revision, setRevision] = useState(0);
  const [listError, setListError] = useState('');
  const [error, setError] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const [revoke, setRevoke] = useState<MerchantKey | null>(null);
  const [name, setName] = useState('');
  const [busy, setBusy] = useState(false);
  const [secret, setSecret] = useState<CreatedMerchantKey | null>(null);
  const [copied, setCopied] = useState(false);
  const [notice, setNotice] = useState('');
  useEffect(() => {
    const controller = new AbortController(); setLoading(true); setListError('');
    listMerchantKeys(controller.signal).then((data) => { if (!controller.signal.aborted) setKeys(data); }).catch((err) => { if (!controller.signal.aborted) setListError(merchantError(err)); }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [revision]);
  const activeCount = keys.filter((key) => key.status === 'ACTIVE').length;
  const canCreate = profile.merchant?.status === 'APPROVED' && !profile.loading && !profile.error && !loading && !listError && activeCount < 5;
  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (busy || !canCreate || !name.trim()) return;
    setBusy(true); setError('');
    try { const result = await createMerchantKey(name.trim()); setSecret(result); setCopied(false); setCreateOpen(false); setName(''); setRevision((n) => n + 1); }
    catch (err) { setError(merchantError(err)); }
    finally { setBusy(false); }
  }
  async function confirmRevoke() {
    if (!revoke || busy) return; setBusy(true); setError('');
    try { await revokeMerchantKey(revoke.id); if (secret?.id === revoke.id) setSecret(null); setRevoke(null); setNotice('API key revoked. Applications using it can no longer authenticate.'); setRevision((n) => n + 1); }
    catch (err) { setError(merchantError(err)); }
    finally { setBusy(false); }
  }
  async function copySecret() {
    if (!secret) return;
    try { await navigator.clipboard.writeText(secret.apiKey); setCopied(true); }
    catch { setNotice('Copy was unavailable. Select the secret and copy it manually.'); }
  }
  return <>
    <PageHeader title="API keys" description="Connect your store to SwiftDrop. Keep every secret on your server." actions={<Button isDisabled={!canCreate || !!secret} iconLeading={<Plus size={16} aria-hidden="true" />} onClick={() => { setError(''); setCreateOpen(true); }}>Create key</Button>} />
    <LoadState {...profile} />
    {profile.merchant && !profile.loading && !profile.error ? <ApprovalNotice merchant={profile.merchant} /> : null}
    {secret ? <Alert tone="success" title="Key created — copy it now"><div className="flex flex-col gap-3"><p>This is the only time the full secret is shown. Store it in your server’s environment, never in client-side code.</p><code className="select-all break-all rounded-control border border-line bg-surface p-3 font-mono text-body-sm text-text">{secret.apiKey}</code><div className="flex flex-wrap gap-3"><Button variant="secondary" iconLeading={<Copy size={16} aria-hidden="true" />} onClick={copySecret}>{copied ? 'Copied' : 'Copy secret'}</Button><Button variant="ghost" onClick={() => setSecret(null)}>I have saved this key</Button></div></div></Alert> : null}
    {notice ? <Alert tone="info" title={notice} /> : null}
    <Section title="Your API keys" description={`Use a separate key for each integration. ${activeCount} of 5 active keys.`}>
      {activeCount >= 5 ? <Alert tone="warning" title="Active key limit reached">Revoke an unused key before creating another.</Alert> : null}
      <LoadState loading={loading} error={listError} retry={() => setRevision((n) => n + 1)} />
      {!loading && !listError ? keys.length ? <div className="overflow-hidden rounded-card border border-line bg-surface"><ul className="divide-y divide-line">{keys.map((key) => <li key={key.id} className="grid gap-4 p-5 md:grid-cols-[1.2fr_1fr_1fr_auto] md:items-center"><div className="min-w-0"><p className="break-words text-label">{key.name}</p><span className={`mt-2 inline-flex rounded-chip border px-2 py-1 text-overline ${key.status === 'ACTIVE' ? 'border-success-border bg-success-tint text-success' : 'border-line bg-surface-2 text-text-muted'}`}>{key.status}</span></div><code className="break-all font-mono text-body-sm">{key.keyPrefix}••••••••</code><dl className="text-caption text-text-muted"><div><dt className="inline">Created: </dt><dd className="inline">{displayDate(key.createdAt)}</dd></div><div><dt className="inline">Last used: </dt><dd className="inline">{displayDate(key.lastUsedAt)}</dd></div></dl>{key.status === 'ACTIVE' ? <Button variant="secondary" aria-label={`Revoke ${key.name}`} onClick={() => { setError(''); setRevoke(key); }}>Revoke</Button> : <span className="text-caption text-text-muted">Revoked</span>}</li>)}</ul></div> : <EmptyState title="No API keys yet" icon={KeyRound}>Once your business is approved, create a key to connect your store.</EmptyState> : null}
    </Section>
    <Modal isOpen={createOpen} onClose={() => setCreateOpen(false)} title="Create API key" description="Choose a name you will recognise, such as Production checkout." isDismissible={!busy}>
      <form onSubmit={create} className="flex flex-col gap-5">{error ? <Alert tone="danger" title="Could not create key">{error}</Alert> : null}<TextField id="merchant-key-name" label="Key name" required maxLength={100} value={name} disabled={busy} onChange={(e) => setName(e.target.value)} data-autofocus /><p className="text-caption text-text-muted">The full secret is shown only once. You can have up to five active keys.</p><div className="flex flex-wrap justify-end gap-3"><Button variant="secondary" isDisabled={busy} onClick={() => setCreateOpen(false)}>Cancel</Button><Button type="submit" isLoading={busy} isDisabled={!name.trim()}>{busy ? 'Creating…' : 'Create key'}</Button></div></form>
    </Modal>
    <Modal isOpen={!!revoke} onClose={() => setRevoke(null)} title={`Revoke ${revoke?.name || 'API key'}?`} description="Applications using this key will stop working immediately. This cannot be undone." isDismissible={!busy} actions={<><Button variant="secondary" isDisabled={busy} onClick={() => setRevoke(null)}>Keep key</Button><Button variant="danger" isLoading={busy} onClick={confirmRevoke}>{busy ? 'Revoking…' : 'Revoke key'}</Button></>}>{error ? <Alert tone="danger" title="Could not revoke key">{error}</Alert> : null}</Modal>
  </>;
}
