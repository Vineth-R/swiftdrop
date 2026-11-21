import type { Page } from '@playwright/test';

import { fail, mockApi, ok, user, type Handler, type Reply } from './mock-api';

export type Role = 'CUSTOMER' | 'MERCHANT' | 'COURIER' | 'DISPATCHER' | 'ADMIN';

export const names: Record<Role, string> = {
  CUSTOMER: 'Nimali Perera',
  MERCHANT: 'Ruwan Jayasuriya',
  COURIER: 'Kasun Silva',
  DISPATCHER: 'Sahan Fernando',
  ADMIN: 'Anusha Nawarathna',
};

/** The API's own words for "this session is over", as opposed to a wrong password. */
const sessionEndCodes = [
  'UNAUTHENTICATED',
  'TOKEN_EXPIRED',
  'REFRESH_TOKEN_INVALID',
  'SESSION_REVOKED',
];

function endsTheSession(reply: Reply): boolean {
  const code = (reply.body as { error?: { code?: string } } | undefined)?.error
    ?.code;
  return (
    reply.status === 401 && code !== undefined && sessionEndCodes.includes(code)
  );
}

/**
 * Signs the page in as `role`, with no backend. `GET /users/me` answers with a
 * user of that role until the page logs out; after that it answers 401. It also
 * answers 401 once any of your own endpoints has said the session is over (a
 * 401 UNAUTHENTICATED, as a real API would stop saying "signed in" too), so
 * the login page the person is sent to does not send them straight back.
 *
 * - `extra` adds your screen's own endpoints, such as 'GET /parcels'. An entry
 *   here wins over the defaults, so you can also replace 'GET /users/me'.
 * - `overrides` changes fields of the signed-in user, such as { phone: '' }.
 *
 * It returns what `mockApi` returns, so `calls` lists every request the page made.
 */
export async function signInAs(
  page: Page,
  role: Role,
  extra: Record<string, Handler> = {},
  overrides: Partial<typeof user> = {},
) {
  let signedIn = true;
  return mockApi(page, {
    'GET /users/me': () =>
      signedIn
        ? ok({
            ...user,
            role,
            fullName: names[role],
            email: `${role.toLowerCase()}@example.com`,
            ...overrides,
          })
        : fail(401, 'UNAUTHENTICATED'),
    'POST /auth/refresh': fail(401, 'REFRESH_TOKEN_INVALID'),
    'POST /auth/logout': () => {
      signedIn = false;
      return { status: 204 };
    },
    ...Object.fromEntries(
      Object.entries(extra).map(([key, handler]) => [
        key,
        ((request) => {
          const reply =
            typeof handler === 'function' ? handler(request) : handler;
          if (endsTheSession(reply)) signedIn = false;
          return reply;
        }) as Handler,
      ]),
    ),
  });
}
