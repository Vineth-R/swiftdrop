'use client';

import { useEffect, useState, type FormEvent } from 'react';

import { ApiError } from '../../lib/api';
import { sendMerchantEmail, type Merchant } from '../../lib/merchant-api';
import { Alert } from '../ui/alert';
import { Button } from '../ui/button';
import { TextField } from '../ui/field';
import { Modal } from '../ui/modal';
import { Textarea } from '../ui/textarea';

const SUBJECT_MAX = 150;
const MESSAGE_MAX = 5000;

function asApiError(error: unknown): ApiError {
  return error instanceof ApiError
    ? error
    : new ApiError({ code: 'INTERNAL', status: 0 });
}

function failureText(error: ApiError) {
  switch (error.code) {
    case 'SERVICE_UNAVAILABLE':
      return 'Merchant email is not configured. Check the SMTP settings.';
    case 'NOT_FOUND':
      return 'This merchant no longer exists. Close the dialog and reload.';
    case 'RATE_LIMITED':
      return error.retryAfter
        ? `Too many requests. Wait ${error.retryAfter} seconds and try again.`
        : 'Too many requests. Wait a moment and try again.';
    case 'NETWORK':
      return 'We could not reach SwiftDrop. Check your connection and try again.';
    default:
      return 'The email could not be sent. Try again in a moment.';
  }
}

type EmailCompositionDialogProps = {
  merchant: Merchant | null;
  onClose: () => void;
  onSent?: () => void;
};

/**
 * Composes an administrative email to a merchant's registered business
 * address. The recipient cannot be changed here, and a successful send is
 * confirmed only after the SMTP server accepts the message.
 */
export function EmailCompositionDialog({
  merchant,
  onClose,
  onSent,
}: EmailCompositionDialogProps) {
  const [subject, setSubject] = useState('');
  const [message, setMessage] = useState('');
  const [subjectError, setSubjectError] = useState('');
  const [messageError, setMessageError] = useState('');
  const [busy, setBusy] = useState(false);
  const [sent, setSent] = useState(false);
  const [error, setError] = useState<ApiError | null>(null);

  useEffect(() => {
    if (!merchant) return;
    setSubject('');
    setMessage('');
    setSubjectError('');
    setMessageError('');
    setBusy(false);
    setSent(false);
    setError(null);
  }, [merchant]);

  const isOpen = merchant !== null;

  function close() {
    if (busy) return;
    onClose();
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!merchant || busy || sent) return;
    const nextSubject = subject.trim();
    const nextMessage = message.trim();
    let invalid = false;
    if (!nextSubject) {
      setSubjectError('Enter a subject.');
      invalid = true;
    } else if (nextSubject.length > SUBJECT_MAX) {
      setSubjectError(`Use ${SUBJECT_MAX} characters or fewer.`);
      invalid = true;
    } else {
      setSubjectError('');
    }
    if (!nextMessage) {
      setMessageError('Enter a message.');
      invalid = true;
    } else if (nextMessage.length > MESSAGE_MAX) {
      setMessageError(`Use ${MESSAGE_MAX} characters or fewer.`);
      invalid = true;
    } else {
      setMessageError('');
    }
    if (invalid) return;

    setBusy(true);
    setError(null);
    try {
      await sendMerchantEmail(merchant.id, {
        subject: nextSubject,
        message: nextMessage,
      });
      setSent(true);
      onSent?.();
    } catch (err) {
      setError(asApiError(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal
      isOpen={isOpen}
      onClose={close}
      title="Compose email"
      description={
        merchant
          ? `Send a message to ${merchant.businessName} at ${merchant.contactEmail}.`
          : undefined
      }
      size="lg"
      isDismissible={!busy}
      actions={
        sent ? (
          <Button onClick={close}>Close</Button>
        ) : (
          <>
            <Button variant="secondary" onClick={close} isDisabled={busy}>
              Cancel
            </Button>
            <Button type="submit" form="merchant-email-form" isLoading={busy}>
              Send email
            </Button>
          </>
        )
      }
    >
      {sent ? (
        <Alert tone="success" title="Email sent">
          The message was accepted for delivery to the merchant's registered
          email address.
        </Alert>
      ) : (
        <form
          id="merchant-email-form"
          noValidate
          onSubmit={submit}
          className="flex flex-col gap-4"
        >
          {error ? (
            <Alert tone="danger" title="Could not send the email">
              {failureText(error)}
            </Alert>
          ) : null}
          <TextField
            id="merchant-email-to"
            label="To"
            value={merchant?.contactEmail ?? ''}
            readOnly
            isMono
            helper="The merchant's registered business email."
          />
          <TextField
            id="merchant-email-subject"
            label="Subject"
            value={subject}
            onChange={(event) => {
              setSubject(event.target.value);
              setSubjectError('');
            }}
            maxLength={SUBJECT_MAX}
            error={subjectError || undefined}
            isRequired
            data-autofocus
          />
          <Textarea
            id="merchant-email-message"
            label="Message"
            value={message}
            onChange={(event) => {
              setMessage(event.target.value);
              setMessageError('');
            }}
            rows={8}
            maxLength={MESSAGE_MAX}
            showCount
            error={messageError || undefined}
            isRequired
            helper="Write the message in plain language. It is sent with the SwiftDrop header and footer."
          />
        </form>
      )}
    </Modal>
  );
}
