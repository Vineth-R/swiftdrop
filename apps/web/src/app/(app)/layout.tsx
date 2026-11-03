import type { ReactNode } from 'react';

import { SignedInOnly } from '../../components/auth/session-gates';
import { RoleShell } from '../../components/layout/role-shell';
import { SessionProvider } from '../../lib/session';

/**
 * Every signed-in screen lives under this group. The session gate shows the
 * splash while the cookie is checked and sends a signed-out visitor to login;
 * the shell then wraps the page for the person's role. Put a new screen in
 * app/(app)/<name>/page.tsx and it gets both for free.
 */
export default function AppGroupLayout({ children }: { children: ReactNode }) {
  return (
    <SessionProvider>
      <SignedInOnly>
        <RoleShell>{children}</RoleShell>
      </SignedInOnly>
    </SessionProvider>
  );
}
