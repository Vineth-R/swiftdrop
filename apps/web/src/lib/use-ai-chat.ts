'use client';

import { useCallback, useSyncExternalStore } from 'react';

import { useAiChatStore } from './ai-chat-context';

export type { ChatMessage } from './ai-chat-store';

/**
 * The assistant conversation, read from the page-session store. The panel is a
 * view over it: closing the panel does not own or destroy anything.
 *
 * - The conversation ID stays inside the store and is sent with every
 *   follow-up; a token or an internal ID never reaches the screen.
 * - A request in flight blocks a second send.
 * - `404 CONVERSATION_NOT_FOUND` (an expired in-process store, or a restarted
 *   process) is handled by starting a new conversation and re-sending once.
 * - An identity change aborts the request and shows nothing from the previous
 *   identity.
 */
export function useAiChat() {
  const store = useAiChatStore();
  const state = useSyncExternalStore(
    store.subscribe,
    store.getSnapshot,
    store.getServerSnapshot,
  );

  const send = useCallback(
    (message: string) => {
      store.send(message);
    },
    [store],
  );

  const retry = useCallback(() => {
    store.retry();
  }, [store]);

  const dismissError = useCallback(() => {
    store.dismissError();
  }, [store]);

  return {
    ...state,
    send,
    retry,
    dismissError,
  };
}
