'use client';

import { Sparkles, X } from 'lucide-react';
import Link from 'next/link';
import {
  useEffect,
  useLayoutEffect,
  useRef,
  type KeyboardEvent as ReactKeyboardEvent,
} from 'react';

import { useAiChatStore } from '../../lib/ai-chat-context';
import { useAiChat } from '../../lib/use-ai-chat';
import { Alert } from '../ui/alert';
import { Button } from '../ui/button';
import { IconButton } from '../ui/icon-button';
import { ChatInput } from './chat-input';
import { ChatMessage } from './chat-message';
import { SuggestedQuestions } from './suggested-questions';
import { TypingIndicator } from './typing-indicator';

const WELCOME =
  "Hi! I'm your SwiftDrop Delivery Assistant. I can help you calculate delivery fees, track parcels, understand our services and find information about your deliveries.";

type ChatPanelProps = {
  onClose: () => void;
};

const FOCUSABLE =
  'button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), a[href], [tabindex]:not([tabindex="-1"])';

/**
 * The conversation panel. It is presented as a dialog: focus moves to the
 * message box on open, Tab stays inside, Escape closes, and the widget then
 * returns focus to the launcher. A signed-in visitor's cookies ride along, so
 * the backend can use private tools; this panel never decides permissions.
 */
export function ChatPanel({ onClose }: ChatPanelProps) {
  const {
    messages,
    isSending,
    error: failure,
    failedMessage,
    send,
    retry,
    retryToken,
    dismissError,
  } = useAiChat();
  const store = useAiChatStore();
  const panelRef = useRef<HTMLDivElement>(null);
  const conversationRef = useRef<HTMLDivElement>(null);
  const previousConversationState = useRef({
    length: messages.length,
    sending: isSending,
  });
  const stickToBottom = useRef(true);
  const restoredPosition = useRef(false);

  useEffect(() => {
    panelRef.current?.querySelector<HTMLTextAreaElement>('textarea')?.focus();
  }, []);

  // The panel can be closed and reopened without touching the conversation.
  // On the first mount of an existing conversation, put the reader back where
  // they were before paint; a fresh conversation stays at the top. The ref
  // makes this a once-per-mount step, so it never fights new messages
  // (including React StrictMode's double effect invoke).
  useLayoutEffect(() => {
    if (restoredPosition.current) return;
    restoredPosition.current = true;
    const conversation = conversationRef.current;
    if (!conversation) return;
    conversation.scrollTop = messages.length === 0 ? 0 : store.getScrollTop();
    const distance =
      conversation.scrollHeight -
      conversation.scrollTop -
      conversation.clientHeight;
    stickToBottom.current = distance <= 48;
  }, [messages.length, store]);

  // Only follow new messages after the first render. The initial welcome and
  // suggested questions must stay at the top, and a reader who scrolled up is
  // never pulled back to the bottom. Comparing with the previous values also
  // keeps the mount-time effect (including React StrictMode's double invoke)
  // from being mistaken for a conversation update.
  useEffect(() => {
    const previous = previousConversationState.current;
    const changed =
      previous.length !== messages.length || previous.sending !== isSending;
    previousConversationState.current = {
      length: messages.length,
      sending: isSending,
    };
    if (!changed || !stickToBottom.current) return;
    const conversation = conversationRef.current;
    if (conversation) {
      conversation.scrollTo({ top: conversation.scrollHeight });
      store.setScrollTop(conversation.scrollTop);
    }
  }, [messages.length, isSending, store]);

  function handleConversationScroll() {
    const conversation = conversationRef.current;
    if (!conversation) return;
    store.setScrollTop(conversation.scrollTop);
    const distance =
      conversation.scrollHeight -
      conversation.scrollTop -
      conversation.clientHeight;
    stickToBottom.current = distance <= 48;
  }

  useEffect(() => {
    if (!failure?.waitSeconds || failure.waitSeconds <= 0) return;
    const timer = window.setTimeout(
      () => dismissError(),
      failure.waitSeconds * 1000,
    );
    return () => window.clearTimeout(timer);
  }, [failure, dismissError]);

  function handleKeyDown(event: ReactKeyboardEvent<HTMLDivElement>) {
    if (event.key === 'Escape') {
      event.preventDefault();
      onClose();
      return;
    }
    if (event.key !== 'Tab') return;

    const panel = panelRef.current;
    if (!panel) return;
    const items = Array.from(
      panel.querySelectorAll<HTMLElement>(FOCUSABLE),
    ).filter((item) => item.tabIndex !== -1);
    if (items.length === 0) return;

    const first = items[0];
    const last = items[items.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  }

  function sendMessage(message: string) {
    dismissError();
    stickToBottom.current = true;
    send(message);
  }

  return (
    <div
      ref={panelRef}
      id="ai-chat-panel"
      role="dialog"
      aria-modal="false"
      aria-labelledby="ai-chat-title"
      onKeyDown={handleKeyDown}
      className="fixed inset-4 z-50 flex flex-col overflow-hidden rounded-card border border-line bg-surface shadow-modal lg:inset-auto lg:top-24 lg:bottom-40 lg:right-4 lg:w-96"
    >
      <header className="flex items-start gap-3 border-b border-line px-4 py-3">
        <span
          aria-hidden="true"
          className="mt-1 inline-flex size-9 shrink-0 items-center justify-center rounded-control bg-primary text-on-primary"
        >
          <Sparkles size={18} strokeWidth={1.5} />
        </span>
        <div className="min-w-0 flex-1">
          <h2 id="ai-chat-title" className="text-h4">
            SwiftDrop Delivery Assistant
          </h2>
          <p className="text-caption text-text-muted">
            Delivery help from SwiftDrop
          </p>
        </div>
        <IconButton
          icon={X}
          label="Close the assistant"
          size="md"
          onClick={onClose}
        />
      </header>

      <div
        ref={conversationRef}
        role="log"
        aria-live="polite"
        aria-relevant="additions"
        aria-label="Conversation"
        onScroll={handleConversationScroll}
        className="flex-1 overflow-y-auto px-4 py-4"
      >
        {messages.length === 0 ? (
          <div className="flex flex-col gap-4">
            <div className="flex flex-col gap-2">
              <p className="text-body-sm text-text">{WELCOME}</p>
              <p className="text-caption text-text-muted">
                I answer questions about SwiftDrop. I cannot help with other
                topics.
              </p>
              <p className="text-caption text-text-muted">
                You can ask as a guest.{' '}
                <Link
                  href="/login"
                  className="underline decoration-line underline-offset-2 hover:text-text"
                >
                  Sign in
                </Link>{' '}
                or{' '}
                <Link
                  href="/register"
                  className="underline decoration-line underline-offset-2 hover:text-text"
                >
                  create an account
                </Link>{' '}
                to see your own deliveries.
              </p>
            </div>
            <SuggestedQuestions onSelect={sendMessage} isDisabled={isSending} />
          </div>
        ) : (
          <div className="flex flex-col gap-3">
            {messages.map((message) => (
              <ChatMessage
                key={message.id}
                role={message.role}
                text={message.text}
                toolCalls={message.toolCalls}
              />
            ))}
            {isSending && <TypingIndicator />}
          </div>
        )}
      </div>

      {failure && (
        <div className="border-t border-line px-4 py-3">
          <Alert
            tone="danger"
            title={failure.title}
            actions={
              failure.canRetry ? (
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={() => {
                    dismissError();
                    retry();
                  }}
                >
                  Try again
                </Button>
              ) : undefined
            }
          >
            <p>{failure.message}</p>
            {failure.reference && (
              <p className="font-mono text-caption">
                Reference {failure.reference}
              </p>
            )}
          </Alert>
        </div>
      )}

      <div className="border-t border-line p-3">
        <ChatInput
          onSend={sendMessage}
          isSending={isSending}
          failedMessage={failedMessage}
          clearToken={retryToken}
          retryAfterSeconds={failure?.waitSeconds}
        />
      </div>
    </div>
  );
}
