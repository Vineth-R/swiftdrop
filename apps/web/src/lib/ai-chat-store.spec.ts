import { afterEach, describe, expect, it, vi } from 'vitest';

import {
  bindIdentityResets,
  createAiChatStore,
  EMPTY_AI_CHAT_STATE,
  type ChatTransport,
} from './ai-chat-store';
import {
  emitSessionIdentity,
  getSessionIdentityVersion,
  subscribeSessionIdentity,
} from './session-events';
import type { ChatReply } from './ai-chat-api';

const reply = (
  text: string,
  conversationId = 'cv00000000000000000000000000000001',
): ChatReply => ({ conversationId, reply: text, toolCalls: [] });

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((settle, fail) => {
    resolve = settle;
    reject = fail;
  });
  return { promise, resolve, reject };
}

const cleanups: Array<() => void> = [];
let identitySequence = 0;

function nextIdentity(prefix = 'user'): string {
  identitySequence += 1;
  return `${prefix}-${identitySequence}`;
}

afterEach(() => {
  while (cleanups.length > 0) cleanups.pop()?.();
});

/** Binds a store to the real session identity events for one test. */
function bind(store: ReturnType<typeof createAiChatStore>) {
  cleanups.push(
    bindIdentityResets(
      store,
      subscribeSessionIdentity,
      getSessionIdentityVersion,
    ),
  );
}

/** Waits for pending microtasks started by a send. */
function flush() {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

describe('AI chat conversation store', () => {
  it('keeps the conversation while no panel is subscribed, and on reopen', async () => {
    const transport = vi.fn<ChatTransport>(async () => reply('Hello back.'));
    const store = createAiChatStore(transport);
    const closedPanel = vi.fn();
    const stop = store.subscribe(closedPanel);

    store.send('Hello');
    await flush();
    expect(store.getSnapshot().messages).toHaveLength(2);

    // Closing the panel removes its subscription; it must not clear anything.
    stop();
    expect(store.getSnapshot().messages).toHaveLength(2);

    const reopenedPanel = vi.fn();
    cleanups.push(store.subscribe(reopenedPanel));
    expect(store.getSnapshot().messages.map((message) => message.text)).toEqual(
      ['Hello', 'Hello back.'],
    );
  });

  it('keeps messages in their original order', async () => {
    let counter = 0;
    const transport: ChatTransport = async ({ message }) => {
      counter += 1;
      return reply(
        `Answer to ${message}`,
        `cv0000000000000000000000000000000${counter}`,
      );
    };
    const store = createAiChatStore(transport);

    store.send('First');
    await flush();
    store.send('Second');
    await flush();

    expect(
      store.getSnapshot().messages.map(({ role, text }) => ({ role, text })),
    ).toEqual([
      { role: 'user', text: 'First' },
      { role: 'assistant', text: 'Answer to First' },
      { role: 'user', text: 'Second' },
      { role: 'assistant', text: 'Answer to Second' },
    ]);
  });

  it('continues the same conversation after the panel is reopened', async () => {
    const transport = vi.fn<ChatTransport>(async () => reply('Same thread.'));
    const store = createAiChatStore(transport);

    store.send('First');
    await flush();

    const stop = store.subscribe(vi.fn());
    stop();
    store.send('Second');
    await flush();

    expect(transport).toHaveBeenCalledTimes(2);
    expect(transport.mock.calls[0][0].conversationId).toBeNull();
    expect(transport.mock.calls[1][0].conversationId).toBe(
      'cv00000000000000000000000000000001',
    );
  });

  it('does not destroy the conversation when subscribers come and go', async () => {
    const store = createAiChatStore(async () => reply('Still here.'));
    store.send('Hello');
    await flush();

    for (let page = 0; page < 3; page += 1) {
      const stop = store.subscribe(vi.fn());
      stop();
    }

    expect(store.getSnapshot().messages).toHaveLength(2);
  });

  it('starts every store instance empty, like a reload or a new tab', async () => {
    const first = createAiChatStore(async () => reply('First tab.'));
    first.send('Hello');
    await flush();

    const second = createAiChatStore(async () => reply('Second tab.'));

    expect(first.getSnapshot().messages).toHaveLength(2);
    expect(second.getSnapshot().messages).toEqual([]);
    second.send('Hello');
    await flush();
    expect(second.getSnapshot().messages).toHaveLength(2);
    expect(first.getSnapshot().messages).toHaveLength(2);
  });

  it('clears the previous conversation on an identity change', async () => {
    const store = createAiChatStore(async () => reply('Guest answer.'));
    bind(store);

    store.send('Guest question');
    await flush();
    expect(store.getSnapshot().messages).toHaveLength(2);

    emitSessionIdentity(nextIdentity());
    expect(store.getSnapshot().messages).toEqual([]);
    expect(store.getSnapshot().error).toBeNull();
    expect(store.getSnapshot().isSending).toBe(false);
  });

  it('clears errors and pending retry state on an identity change', async () => {
    const store = createAiChatStore(async () =>
      Promise.reject(new Error('offline')),
    );
    bind(store);

    store.send('Question');
    await flush();
    expect(store.getSnapshot().error).not.toBeNull();
    expect(store.getSnapshot().failedMessage).toBe('Question');

    emitSessionIdentity(nextIdentity());
    expect(store.getSnapshot()).toEqual(EMPTY_AI_CHAT_STATE);
  });

  it('does not clear on a repeated identity event', async () => {
    const identity = nextIdentity('repeated');
    const store = createAiChatStore(async () => reply('Stable.'));
    bind(store);
    emitSessionIdentity(identity);
    store.send('Question');
    await flush();

    emitSessionIdentity(identity);

    expect(store.getSnapshot().messages).toHaveLength(2);
  });

  it('aborts an in-flight turn during an identity change and ignores its reply', async () => {
    const pending = deferred<ChatReply>();
    let signal: AbortSignal | undefined;
    const store = createAiChatStore((_input, requestSignal) => {
      signal = requestSignal;
      return pending.promise;
    });
    bind(store);

    store.send('Guest question');
    expect(signal?.aborted).toBe(false);

    emitSessionIdentity(nextIdentity('inflight'));
    expect(signal?.aborted).toBe(true);

    pending.resolve(reply('Late guest answer.'));
    await flush();

    expect(store.getSnapshot().messages).toEqual([]);
    expect(store.getSnapshot().isSending).toBe(false);
  });

  it('does not merge a guest conversation into the signed-in one', async () => {
    let conversationId: string | null = null;
    const transport = vi.fn<ChatTransport>(async (input) => {
      conversationId = input.conversationId;
      return reply('Answer.', 'cv00000000000000000000000000000002');
    });
    const store = createAiChatStore(transport);
    bind(store);

    store.send('Guest question');
    await flush();
    expect(conversationId).toBeNull();
    expect(store.getSnapshot().messages).toHaveLength(2);

    emitSessionIdentity(nextIdentity('customer'));
    expect(store.getSnapshot().messages).toEqual([]);

    store.send('Customer question');
    await flush();
    expect(transport.mock.calls.at(-1)?.[0].conversationId).toBeNull();
    expect(store.getSnapshot().messages.map((message) => message.text)).toEqual(
      ['Customer question', 'Answer.'],
    );
  });

  it('never exposes customer A messages to customer B', async () => {
    const store = createAiChatStore(async () => reply('Private to A.'));
    bind(store);

    emitSessionIdentity(nextIdentity('customer'));
    store.send('A private question');
    await flush();
    expect(store.getSnapshot().messages).toHaveLength(2);

    emitSessionIdentity(nextIdentity('other'));
    expect(store.getSnapshot().messages).toEqual([]);
  });

  it('restores and resets the saved scroll position', () => {
    const store = createAiChatStore(async () => reply('Answer.'));

    expect(store.getScrollTop()).toBe(0);
    store.setScrollTop(420);
    expect(store.getScrollTop()).toBe(420);
    store.setScrollTop(-10);
    expect(store.getScrollTop()).toBe(0);
    store.setScrollTop(Number.NaN);
    expect(store.getScrollTop()).toBe(0);

    store.setScrollTop(300);
    store.reset();
    expect(store.getScrollTop()).toBe(0);
  });

  it('keeps the failed draft and retries without a second user bubble', async () => {
    let attempt = 0;
    const store = createAiChatStore(async () =>
      attempt++ === 0
        ? Promise.reject(new Error('offline'))
        : reply('Recovered.'),
    );

    store.send('Question');
    await flush();
    expect(store.getSnapshot().messages).toHaveLength(1);
    expect(store.getSnapshot().failedMessage).toBe('Question');

    store.retry();
    await flush();
    expect(
      store.getSnapshot().messages.map(({ role, text }) => ({ role, text })),
    ).toEqual([
      { role: 'user', text: 'Question' },
      { role: 'assistant', text: 'Recovered.' },
    ]);
    expect(store.getSnapshot().failedMessage).toBeNull();
  });
});
