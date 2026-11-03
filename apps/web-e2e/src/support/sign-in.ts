import type { Page } from '@playwright/test';

import { fail, mockApi, ok, user, type Handler } from './mock-api';

export type Role = 'CUSTOMER' | 'MERCHANT' | 'COURIER' | 'DISPATCHER' | 'ADMIN';

export const names: Record<Role, string> = {
  CUSTOMER: 'Nimali Perera',
  MERCHANT: 'Ruwan Jayasuriya',
  COURIER: 'Kasun Silva',
  DISPATCHER: 'Sahan Fernando',
  ADMIN: 'Anusha Nawarathna',
};

/**
 * Signs the page in as `role`, with no backend. `GET /users/me` answers with a
 * user of that role until the page logs out; after that it answers 401.
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
    ...extra,
  });
}
