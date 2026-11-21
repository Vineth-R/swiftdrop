'use client';

import { createContext, useContext, useEffect, type ReactNode } from 'react';

import {
  chatStore,
  bindIdentityResets,
  type AiChatStore,
} from './ai-chat-store';
import {
  getSessionIdentityVersion,
  subscribeSessionIdentity,
} from './session-events';

const AiChatStoreContext = createContext<AiChatStore>(chatStore);

/**
 * Keeps the assistant conversation alive at the application root, outside the
 * panel that shows it. The root layout is mounted once for the page session, so
 * closing the panel or navigating between pages never clears the conversation.
 *
 * Identity changes are handled synchronously at the event source: the store
 * aborts any in-flight request and clears every message before React renders
 * the next identity, so a previous guest or account conversation can never be
 * shown again.
 */
export function AiChatProvider({ children }: { children: ReactNode }) {
  useEffect(
    () =>
      bindIdentityResets(
        chatStore,
        subscribeSessionIdentity,
        getSessionIdentityVersion,
      ),
    [],
  );

  return <AiChatStoreContext value={chatStore}>{children}</AiChatStoreContext>;
}

export function useAiChatStore(): AiChatStore {
  return useContext(AiChatStoreContext);
}
