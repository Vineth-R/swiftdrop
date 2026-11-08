'use client';

import { SendHorizontal } from 'lucide-react';
import {
  useEffect,
  useRef,
  useState,
  type FormEvent,
  type KeyboardEvent,
} from 'react';

import { CHAT_MESSAGE_MAX } from '../../lib/ai-chat-api';
import { IconButton } from '../ui/icon-button';
import { TextareaControl } from '../ui/textarea-control';

type ChatInputProps = {
  onSend: (message: string) => void;
  isSending: boolean;
  /** The last message that failed, so the input can keep it for a retry. */
  failedMessage?: string | null;
  /** From a 429. The Send control stays off until the wait has passed. */
  retryAfterSeconds?: number;
};

/** Counts the rate-limit wait down, once per second, without a looping animation. */
function useWaitSeconds(retryAfterSeconds?: number): number {
  const [remaining, setRemaining] = useState(0);

  useEffect(() => {
    if (!retryAfterSeconds || retryAfterSeconds <= 0) {
      setRemaining(0);
      return;
    }
    setRemaining(retryAfterSeconds);
    const timer = window.setInterval(() => {
      setRemaining((current) => {
        if (current <= 1) {
          window.clearInterval(timer);
          return 0;
        }
        return current - 1;
      });
    }, 1000);
    return () => window.clearInterval(timer);
  }, [retryAfterSeconds]);

  return remaining;
}

/**
 * Enter sends, Shift+Enter starts a new line, and an empty or whitespace-only
 * message never leaves the panel. The text stays in the box until a send is
 * accepted, so a failed request does not lose what was typed.
 */
export function ChatInput({
  onSend,
  isSending,
  failedMessage,
  retryAfterSeconds,
}: ChatInputProps) {
  const [value, setValue] = useState('');
  const lastSent = useRef('');
  const wasSending = useRef(false);
  const remaining = useWaitSeconds(retryAfterSeconds);
  const trimmed = value.trim();
  const canSend = trimmed.length > 0 && !isSending && remaining === 0;

  useEffect(() => {
    if (wasSending.current && !isSending) {
      if (!failedMessage && value === lastSent.current) {
        setValue('');
        lastSent.current = '';
      }
    }
    wasSending.current = isSending;
  }, [failedMessage, isSending, value]);

  function submit() {
    if (!canSend) return;
    lastSent.current = trimmed;
    onSend(trimmed);
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    submit();
  }

  function handleKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (
      event.key === 'Enter' &&
      !event.shiftKey &&
      !event.nativeEvent.isComposing
    ) {
      event.preventDefault();
      submit();
    }
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-1">
      <div className="flex items-end gap-2">
        <label htmlFor="ai-chat-message" className="sr-only">
          Message the SwiftDrop Delivery Assistant
        </label>
        <TextareaControl
          id="ai-chat-message"
          rows={1}
          maxLength={CHAT_MESSAGE_MAX}
          value={value}
          onChange={(event) => setValue(event.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Ask about a delivery, price or tracking"
          className="max-h-24 min-h-0 resize-none"
        />
        <IconButton
          icon={SendHorizontal}
          label="Send message"
          variant="secondary"
          type="submit"
          isLoading={isSending}
          isDisabled={trimmed.length === 0 || remaining > 0}
        />
      </div>
      <div className="flex items-center justify-between gap-2 px-1">
        <span className="text-caption text-text-muted">
          {remaining > 0
            ? `Try again in ${remaining} seconds`
            : 'Enter to send, Shift+Enter for a new line'}
        </span>
        <span className="font-mono text-caption tabular text-text-muted">
          {value.length}/{CHAT_MESSAGE_MAX}
        </span>
      </div>
    </form>
  );
}
