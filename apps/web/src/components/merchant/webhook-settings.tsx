'use client';

import { Copy } from 'lucide-react';
import { useEffect, useState, type FormEvent } from 'react';

import { ApiError } from '../../lib/api';
import { useForm, type FormRules } from '../../lib/use-form';
import {
  WEBHOOK_EVENT,
  getWebhook,
  saveWebhook,
  validateWebhookUrl,
  type WebhookSettings as Settings,
} from '../../lib/webhook-api';
import { Alert } from '../ui/alert';
import { Button } from '../ui/button';
import { Checkbox, CheckboxGroup } from '../ui/checkbox';
import { TextField } from '../ui/field';
import { Skeleton, SkeletonGroup } from '../ui/skeleton';
import { WebhookProblem, useWebhookProblem } from './webhook-problem';
import { WebhookTestEvent } from './webhook-test-event';

type Values = { url: string };

// Defined outside the component so useForm sees the same object every render.
const rules: FormRules<Values> = { url: (v) => validateWebhookUrl(v.url) };

const URL_REFUSED =
  'SwiftDrop cannot send to this address. Use a public https:// address, not localhost or a private network.';

const eventOptions = [
  {
    value: WEBHOOK_EVENT,
    label: 'Parcel status changed',
    description: 'Sent each time one of your parcels moves to a new status.',
    isDisabled: true,
  },
];

/** The signing secret, shown once. It cannot be read again after this. */
function SecretNotice({
  secret,
  onDone,
  onCopyFailed,
}: {
  secret: string;
  onDone: () => void;
  onCopyFailed: () => void;
}) {
  const [copied, setCopied] = useState(false);

  async function copy() {
    try {
      await navigator.clipboard.writeText(secret);
      setCopied(true);
    } catch {
      onCopyFailed();
    }
  }

  return (
    <Alert tone="success" title="Signing secret: copy it now">
      <div className="flex flex-col gap-3">
        <p>
          This is the only time the full secret is shown. Keep it on your server
          and use it to check the signature of each request.
        </p>
        <code className="select-all break-all rounded-control border border-line bg-surface p-3 font-mono text-body-sm text-text">
          {secret}
        </code>
        <div className="flex flex-wrap gap-3">
          <Button
            variant="secondary"
            iconLeading={
              <Copy size={16} strokeWidth={1.5} aria-hidden="true" />
            }
            onClick={copy}
          >
            {copied ? 'Copied' : 'Copy secret'}
          </Button>
          <Button variant="ghost" onClick={onDone}>
            I have saved this secret
          </Button>
        </div>
      </div>
    </Alert>
  );
}

/** The form itself. `initial` is null until the merchant saves a webhook. */
function WebhookForm({ initial }: { initial: Settings | null }) {
  const [saved, setSaved] = useState(initial);
  const form = useForm<Values>({ url: initial?.url ?? '' }, rules);
  const [isActive, setIsActive] = useState(initial?.isActive ?? true);
  const [rotate, setRotate] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [isDone, setIsDone] = useState(false);
  const [secret, setSecret] = useState<string | null>(null);
  const [notice, setNotice] = useState('');
  const { problem, fail, clear, alertRef } = useWebhookProblem();

  const isDirty =
    !saved ||
    form.values.url.trim() !== saved.url ||
    isActive !== saved.isActive ||
    rotate;

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isSaving) return;
    clear();
    setIsDone(false);

    if (form.validate().url) {
      document.getElementById('webhook-url')?.focus();
      return;
    }

    setIsSaving(true);
    try {
      const result = await saveWebhook({
        url: form.values.url.trim(),
        eventTypes: [WEBHOOK_EVENT],
        isActive,
        rotateSecret: rotate,
      });
      const { signingSecret, ...settings } = result;
      setSaved(settings);
      form.setValue('url', settings.url);
      form.resetValidation();
      setRotate(false);
      setSecret(signingSecret ?? null);
      setNotice('');
      setIsDone(true);
    } catch (error) {
      if (error instanceof ApiError && error.code === 'VALIDATION_FAILED') {
        form.setServerErrors({ url: URL_REFUSED });
      } else {
        fail(error);
      }
    } finally {
      setIsSaving(false);
    }
  }

  return (
    <div className="flex flex-col gap-6">
      {secret && (
        <SecretNotice
          secret={secret}
          onDone={() => setSecret(null)}
          onCopyFailed={() =>
            setNotice(
              'Copy was unavailable. Select the secret and copy it by hand.',
            )
          }
        />
      )}
      {notice && <Alert tone="info" title={notice} />}
      {isDone && !secret && (
        <Alert tone="success" title="Webhook saved">
          Your changes are on your account.
        </Alert>
      )}
      {problem && <WebhookProblem problem={problem} alertRef={alertRef} />}

      <form
        noValidate
        onSubmit={submit}
        className="flex max-w-xl flex-col gap-5"
      >
        <TextField
          id="webhook-url"
          label="Address"
          type="url"
          autoComplete="off"
          isRequired
          isMono
          maxLength={500}
          helper="A public https:// address on your server. SwiftDrop posts each event here."
          value={form.values.url}
          readOnly={isSaving}
          error={form.errors.url}
          onChange={(event) => {
            setIsDone(false);
            form.setValue('url', event.target.value);
          }}
          onBlur={() => form.blur('url')}
        />
        <CheckboxGroup
          name="webhook-events"
          legend="Events"
          options={eventOptions}
          values={[WEBHOOK_EVENT]}
          onChange={() => undefined}
        />
        <Checkbox
          id="webhook-active"
          label="Send events to this address"
          description="Untick to pause deliveries. Your address and secret are kept."
          checked={isActive}
          disabled={isSaving}
          onChange={(event) => {
            setIsDone(false);
            setIsActive(event.target.checked);
          }}
        />
        {saved && (
          <Checkbox
            id="webhook-rotate"
            label="Generate a new signing secret"
            description="The current secret stops working when you save. Your server must switch to the new one straight away."
            checked={rotate}
            disabled={isSaving}
            onChange={(event) => {
              setIsDone(false);
              setRotate(event.target.checked);
            }}
          />
        )}
        <div className="flex flex-wrap items-center gap-3">
          <Button type="submit" isLoading={isSaving} isDisabled={!isDirty}>
            {isSaving ? 'Saving' : 'Save webhook'}
          </Button>
          {saved && !isDirty && !isDone && (
            <span className="text-caption text-text-faint">
              Change something to save.
            </span>
          )}
        </div>
      </form>

      <WebhookTestEvent isConfigured={saved !== null} />

      <p className="max-w-prose text-body-sm text-text-muted">
        Check the X-SwiftDrop-Signature header with your signing secret before
        you trust a request. The same event can arrive more than once, so use
        its event ID to ignore repeats.
      </p>
    </div>
  );
}

type Load =
  | { status: 'loading' }
  | { status: 'ready'; settings: Settings | null }
  | { status: 'error' };

/** Loads the saved settings, then shows the form. A 404 means none are saved yet. */
export function WebhookSettings() {
  const [load, setLoad] = useState<Load>({ status: 'loading' });
  const [revision, setRevision] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    setLoad({ status: 'loading' });
    getWebhook(controller.signal)
      .then((settings) => setLoad({ status: 'ready', settings }))
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        if (error instanceof ApiError && error.code === 'NOT_FOUND') {
          setLoad({ status: 'ready', settings: null });
        } else {
          setLoad({ status: 'error' });
        }
      });
    return () => controller.abort();
  }, [revision]);

  if (load.status === 'loading') {
    return (
      <SkeletonGroup
        label="Loading webhook settings"
        className="flex max-w-xl flex-col gap-4"
      >
        <Skeleton className="h-10 w-full" />
        <Skeleton className="h-5 w-1/2" />
        <Skeleton className="h-10 w-1/3" />
      </SkeletonGroup>
    );
  }
  if (load.status === 'error') {
    return (
      <Alert
        tone="danger"
        title="Could not load your webhook settings"
        actions={
          <Button variant="secondary" onClick={() => setRevision((n) => n + 1)}>
            Try again
          </Button>
        }
      >
        Check your connection and try again.
      </Alert>
    );
  }
  return <WebhookForm initial={load.settings} />;
}
