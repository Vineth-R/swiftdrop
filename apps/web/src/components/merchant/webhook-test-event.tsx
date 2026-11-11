'use client';

import { Send } from 'lucide-react';
import { useState } from 'react';

import { testWebhook, type WebhookTestResult } from '../../lib/webhook-api';
import { Alert } from '../ui/alert';
import { Button } from '../ui/button';
import { WebhookProblem, useWebhookProblem } from './webhook-problem';

/** What the test result means for the merchant, in plain words. */
function ResultAlert({ result }: { result: WebhookTestResult }) {
  if (result.delivered) {
    return (
      <Alert tone="success" title="Test event delivered">
        Your server answered HTTP {result.httpStatus} in {result.durationMs} ms.
      </Alert>
    );
  }
  if (result.httpStatus === null) {
    return (
      <Alert tone="warning" title="No answer from your server">
        We could not connect, or nothing came back within 10 seconds. Check the
        address and that your server is reachable from the internet.
      </Alert>
    );
  }
  return (
    <Alert
      tone="warning"
      title={`Your server answered HTTP ${result.httpStatus}`}
    >
      Only a 2xx answer counts as delivered. A real event would be retried.
      Check your server logs, then try again.
    </Alert>
  );
}

/**
 * Sends a signed sample event to the saved address. It tests what is saved,
 * not what is typed, so it stays off until a webhook exists.
 */
export function WebhookTestEvent({ isConfigured }: { isConfigured: boolean }) {
  const [isSending, setIsSending] = useState(false);
  const [result, setResult] = useState<WebhookTestResult | null>(null);
  const { problem, fail, clear, alertRef } = useWebhookProblem();

  async function send() {
    if (isSending) return;
    setIsSending(true);
    setResult(null);
    clear();
    try {
      setResult(await testWebhook());
    } catch (error) {
      fail(error);
    } finally {
      setIsSending(false);
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-3">
        <Button
          variant="secondary"
          iconLeading={<Send size={16} strokeWidth={1.5} aria-hidden="true" />}
          isLoading={isSending}
          isDisabled={!isConfigured}
          onClick={send}
        >
          {isSending ? 'Sending' : 'Send test event'}
        </Button>
        <span className="text-caption text-text-muted">
          {isConfigured
            ? 'Sends a signed sample event to your saved address.'
            : 'Save an address first.'}
        </span>
      </div>
      {result && <ResultAlert result={result} />}
      {problem && <WebhookProblem problem={problem} alertRef={alertRef} />}
    </div>
  );
}
