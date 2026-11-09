import {
  describeChatFailure,
  isConversationGone,
  sendChatMessage,
  type ChatFailure,
  type ChatReply,
  type ChatToolCall,
} from './ai-chat-api';

export type ChatMessage = {
  id: string;
  role: 'user' | 'assistant';
  text: string;
  /** Only a safe status per tool, never the tool's data. */
  toolCalls?: ChatToolCall[];
};

export type AiChatState = {
  messages: ChatMessage[];
  isSending: boolean;
  error: ChatFailure | null;
  failedMessage: string | null;
  retryToken: number;
};

export type ChatTransport = (
  input: { message: string; conversationId: string | null },
  signal?: AbortSignal,
) => Promise<ChatReply>;

export type AiChatStore = {
  subscribe(listener: () => void): () => void;
  getSnapshot(): AiChatState;
  getServerSnapshot(): AiChatState;
  send(message: string): void;
  retry(): void;
  dismissError(): void;
  /** Clears every piece of conversation state for a new identity. */
  reset(): void;
  getScrollTop(): number;
  setScrollTop(top: number): void;
};

const EMPTY_STATE: AiChatState = Object.freeze({
  messages: [],
  isSending: false,
  error: null,
  failedMessage: null,
  retryToken: 0,
});

/**
 * The assistant conversation for the lifetime of one browser page session.
 *
 * The store deliberately lives outside React. A panel can close, reopen or be
 * replaced by navigation without touching the messages or the conversation
 * ID, and no component owns the network request. The only thing that resets
 * the conversation is a page reload/new tab (a fresh module instance) or an
 * explicit identity change, which aborts any in-flight turn and clears
 * everything before the next render.
 *
 * Conversation IDs and message text stay in memory only; nothing is written
 * to storage. The backend guest cookie and ownership checks remain the
 * authority, and no identity value is ever sent to the assistant.
 */
export function createAiChatStore(
  transport: ChatTransport = sendChatMessage,
): AiChatStore {
  let state: AiChatState = EMPTY_STATE;
  let conversationId: string | null = null;
  let attempt: { message: string } | null = null;
  let abortController: AbortController | null = null;
  let isSending = false;
  let messageCount = 0;
  // A monotonically increasing turn marker. Identity resets bump it so a late
  // response from the previous identity can never be appended.
  let generation = 0;
  let scrollTop = 0;
  const listeners = new Set<() => void>();

  function notify() {
    listeners.forEach((listener) => listener());
  }

  function setState(next: AiChatState) {
    state = next;
    notify();
  }

  function patch(update: Partial<AiChatState>) {
    setState({ ...state, ...update });
  }

  function nextId(): string {
    messageCount += 1;
    return `chat-${messageCount}`;
  }

  async function run(rawMessage: string, appendUserMessage: boolean) {
    const text = rawMessage.trim();
    if (!text || isSending) return;

    isSending = true;
    attempt = { message: text };
    const turn = generation;
    const controller = new AbortController();
    abortController = controller;

    patch({ isSending: true, error: null, failedMessage: null });
    if (appendUserMessage) {
      patch({
        messages: [...state.messages, { id: nextId(), role: 'user', text }],
      });
    }

    try {
      let reply: ChatReply;
      try {
        reply = await transport(
          { message: text, conversationId },
          controller.signal,
        );
      } catch (caught) {
        // A conversation the in-process store no longer has (for example
        // after a restart) is not a failure: begin a new one and retry once.
        if (turn !== generation || controller.signal.aborted) return;
        if (conversationId !== null && isConversationGone(caught)) {
          conversationId = null;
          reply = await transport(
            { message: text, conversationId: null },
            controller.signal,
          );
        } else {
          throw caught;
        }
      }

      if (turn !== generation || controller.signal.aborted) return;

      conversationId = reply.conversationId;
      attempt = null;
      patch({
        messages: [
          ...state.messages,
          {
            id: nextId(),
            role: 'assistant',
            text: reply.reply,
            toolCalls: reply.toolCalls,
          },
        ],
        isSending: false,
        failedMessage: null,
      });
    } catch (caught) {
      if (turn !== generation || controller.signal.aborted) return;
      patch({
        isSending: false,
        failedMessage: text,
        error: describeChatFailure(caught),
      });
    } finally {
      if (turn === generation) {
        isSending = false;
        if (abortController === controller) abortController = null;
      }
    }
  }

  return {
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    getSnapshot() {
      return state;
    },
    getServerSnapshot() {
      return EMPTY_STATE;
    },
    send(message) {
      void run(message, true);
    },
    retry() {
      if (!attempt || isSending) return;
      patch({ retryToken: state.retryToken + 1 });
      void run(attempt.message, false);
    },
    dismissError() {
      if (state.error === null && state.failedMessage === null) return;
      patch({ error: null, failedMessage: null });
    },
    reset() {
      generation += 1;
      abortController?.abort();
      abortController = null;
      conversationId = null;
      attempt = null;
      isSending = false;
      messageCount = 0;
      scrollTop = 0;
      setState(EMPTY_STATE);
    },
    getScrollTop() {
      return scrollTop;
    },
    setScrollTop(top) {
      scrollTop = Number.isFinite(top) && top > 0 ? top : 0;
    },
  };
}

/**
 * The one conversation store for this page. The module is evaluated once per
 * browser tab, so a refresh or a new tab starts empty while navigation between
 * pages keeps the same store.
 */
export const chatStore = createAiChatStore();

/**
 * Clears `store` whenever the authentication identity changes. The subscription
 * callback runs synchronously at the event source, so stale messages are gone
 * before React renders the new identity.
 */
export function bindIdentityResets(
  store: AiChatStore,
  subscribe: (listener: () => void) => () => void,
  getVersion: () => number,
): () => void {
  let seen = getVersion();
  return subscribe(() => {
    const current = getVersion();
    if (current === seen) return;
    seen = current;
    store.reset();
  });
}

export { EMPTY_STATE as EMPTY_AI_CHAT_STATE };
