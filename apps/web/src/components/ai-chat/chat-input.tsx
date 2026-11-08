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
  /** Increments when a retry re-sends the failed message; clears the draft. */
  clearToken?: number;
  /** From a 429. The Send control stays off until the wait has passed. */
  retryAfterSeconds?: number;
};

/** Counts Unicode code points, not UTF-16 code units, so emoji count once. */
function countRunes(value: string): number {
  return Array.from(value).length;
}

function clampToRunes(value: string, limit: number): string {
  const characters = Array.from(value);
  return characters.length > limit
    ? characters.slice(0, limit).join('')
    : value;
}

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
 * message never leaves the panel. The composer clears as soon as the send is
 * accepted, so the message is not left behind while the assistant answers. A
 * failed request restores the draft only if the composer is still empty.
 */
export function ChatInput({
  onSend,
  isSending,
  failedMessage,
  clearToken,
  retryAfterSeconds,
}: ChatInputProps) {
  const [value, setValue] = useState('');
  const remaining = useWaitSeconds(retryAfterSeconds);
  const lastClearToken = useRef(clearToken ?? 0);
  const trimmed = value.trim();
  const length = countRunes(value);
  const canSend = trimmed.length > 0 && !isSending && remaining === 0;

  useEffect(() => {
    if (clearToken === undefined || clearToken === lastClearToken.current)
      return;
    lastClearToken.current = clearToken;
    setValue('');
  }, [clearToken]);

  useEffect(() => {
    if (!failedMessage) return;
    setValue((current) => (current === '' ? failedMessage : current));
  }, [failedMessage]);

  function submit() {
    if (!canSend) return;
    const message = trimmed;
    setValue('');
    onSend(message);
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
          value={value}
          onChange={(event) =>
            setValue(clampToRunes(event.target.value, CHAT_MESSAGE_MAX))
          }
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
          {length}/{CHAT_MESSAGE_MAX}
        </span>
      </div>
    </form>
  );
}
