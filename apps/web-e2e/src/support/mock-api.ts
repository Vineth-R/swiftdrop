import type { Page } from '@playwright/test';

/**
 * Canned answers for the API, so a screen can be tested in every state without
 * a backend. Shapes follow API v1.1: a success is { data, meta }, a failure is
 * { error: { code, message, details }, meta }. The server message is
 * deliberately loud so a test fails if a screen ever prints it.
 */

export type Reply = {
  status: number;
  body?: unknown;
  headers?: Record<string, string>;
  delayMs?: number;
};
export type Handler =
  | Reply
  | ((request: { body: unknown; url: URL; query: URLSearchParams }) => Reply);

export const SERVER_WORDS = 'RAW SERVER WORDS THAT MUST NEVER BE SHOWN';
export const REQUEST_ID = '7f2c9a14-0b63-4de8-9f10-5c2e8d3a1b77';

export const user = {
  id: '3f1c7a52-0000-4000-8000-0000000000c3',
  email: 'nimal@example.com',
  fullName: 'Nimal Perera',
  phone: '+94771234567',
  role: 'CUSTOMER',
  authProvider: 'LOCAL',
  createdAt: '2026-11-02T09:30:00Z',
};

export const ok = (data: unknown, status = 200): Reply => ({
  status,
  body: {
    data,
    meta: { requestId: REQUEST_ID, timestamp: '2026-11-02T09:30:00Z' },
  },
});

export const fail = (
  status: number,
  code: string,
  extra: { details?: unknown; headers?: Record<string, string> } = {},
): Reply => ({
  status,
  headers: extra.headers,
  body: {
    error: { code, message: SERVER_WORDS, details: extra.details },
    meta: { requestId: REQUEST_ID },
  },
});

export const signedOut: Record<string, Handler> = {
  'GET /users/me': fail(401, 'UNAUTHENTICATED'),
  'POST /auth/refresh': fail(401, 'REFRESH_TOKEN_INVALID'),
};

/** Answers every /api/v1 call from `handlers`; anything else is a 404. */
export async function mockApi(
  page: Page,
  handlers: Record<string, Handler>,
): Promise<{
  calls: Array<{ key: string; body: unknown; url: string }>;
}> {
  const calls: Array<{ key: string; body: unknown; url: string }> = [];
  await page.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace('/api/v1', '');
    const key = `${request.method()} ${path}`;
    const rawBody = request.postData();
    let body: unknown;
    if (rawBody) {
      try {
        body = request.postDataJSON();
      } catch {
        body = rawBody;
      }
    }
    calls.push({ key, body, url: request.url() });

    const handler = handlers[key] ?? fail(404, 'NOT_FOUND');
    const url = new URL(request.url());
    const reply =
      typeof handler === 'function'
        ? handler({ body, url, query: url.searchParams })
        : handler;
    if (reply.delayMs) await new Promise((r) => setTimeout(r, reply.delayMs));
    await route.fulfill({
      status: reply.status,
      contentType: 'application/json',
      headers: reply.headers,
      body: reply.body === undefined ? '' : JSON.stringify(reply.body),
    });
  });
  return { calls };
}
