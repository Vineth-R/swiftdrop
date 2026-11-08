'use client';

import { useCallback, useEffect, useRef, useState } from 'react';

import {
  describeChatFailure,
  isConversationGone,
  sendChatMessage,
  type ChatFailure,
  type ChatToolCall,
} from './ai-chat-api';

export type ChatMessage = {
  id: string;
  role: 'user' | 'assistant';
  text: string;
  /** Only a safe status per tool, never the tool's data. */
  toolCalls?: ChatToolCall[];
};

type LastAttempt = { message: string };

/**
 * The assistant conversation for one mounted panel.
 *
 * - The conversation ID lives in a ref: it is sent with every follow-up and is
 *   not rendered, so a token or an internal ID never reaches the screen.
 * - A request in flight blocks a second send.
 * - `404 CONVERSATION_NOT_FOUND` (an expired in-process store, or a restarted
 *   process) is handled by starting a new conversation and re-sending once.
 * - Closing the panel aborts the request, and an aborted request shows nothing.
 */
export function useAiChat() {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [isSending, setIsSending] = useState(false);
  const [error, setError] = useState<ChatFailure | null>(null);
  const [failedMessage, setFailedMessage] = useState<string | null>(null);

  const conversationId = useRef<string | null>(null);
  const attempt = useRef<LastAttempt | null>(null);
  const abortController = useRef<AbortController | null>(null);
  const isSendingRef = useRef(false);
  const isMounted = useRef(true);
  const messageCount = useRef(0);

  useEffect(() => {
    isMounted.current = true;
    return () => {
      isMounted.current = false;
      abortController.current?.abort();
    };
  }, []);

  const nextId = useCallback(() => {
    messageCount.current += 1;
    return `chat-${messageCount.current}`;
  }, []);

  const run = useCallback(
    async (rawMessage: string, appendUserMessage: boolean) => {
      const text = rawMessage.trim();
      if (!text || isSendingRef.current) return;

      isSendingRef.current = true;
      attempt.current = { message: text };
      setError(null);
      setFailedMessage(null);
      setIsSending(true);

      const controller = new AbortController();
      abortController.current = controller;

      if (appendUserMessage) {
        setMessages((previous) => [
          ...previous,
          { id: nextId(), role: 'user', text },
        ]);
      }

      try {
        let reply;
        try {
          reply = await sendChatMessage(
            { message: text, conversationId: conversationId.current },
            controller.signal,
          );
        } catch (caught) {
          // A conversation the in-process store no longer has (for example
          // after a restart) is not a failure: begin a new one and retry once.
          if (
            !controller.signal.aborted &&
            conversationId.current !== null &&
            isConversationGone(caught)
          ) {
            conversationId.current = null;
            reply = await sendChatMessage(
              { message: text, conversationId: null },
              controller.signal,
            );
          } else {
            throw caught;
          }
        }

        if (!isMounted.current || controller.signal.aborted) return;

        conversationId.current = reply.conversationId;
        attempt.current = null;
        setFailedMessage(null);
        setMessages((previous) => [
          ...previous,
          {
            id: nextId(),
            role: 'assistant',
            text: reply.reply,
            toolCalls: reply.toolCalls,
          },
        ]);
      } catch (caught) {
        if (!isMounted.current || controller.signal.aborted) return;
        setFailedMessage(text);
        setError(describeChatFailure(caught));
      } finally {
        if (isMounted.current) setIsSending(false);
        isSendingRef.current = false;
        if (abortController.current === controller)
          abortController.current = null;
      }
    },
    [nextId],
  );

  const send = useCallback(
    (message: string) => {
      void run(message, true);
    },
    [run],
  );

  /** Re-sends the last message without adding a second user bubble. */
  const retry = useCallback(() => {
    if (!attempt.current) return;
    void run(attempt.current.message, false);
  }, [run]);

  const dismissError = useCallback(() => {
    setError(null);
    setFailedMessage(null);
  }, []);

  return {
    messages,
    isSending,
    error,
    failedMessage,
    send,
    retry,
    dismissError,
  };
}
