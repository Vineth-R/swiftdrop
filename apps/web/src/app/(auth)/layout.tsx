import { Suspense, type ReactNode } from 'react';

import { SignedOutOnly } from '../../components/auth/session-gates';
import { SessionProvider } from '../../lib/session';

export default function AuthGroupLayout({ children }: { children: ReactNode }) {
  return (
    <SessionProvider>
      <Suspense>
        <SignedOutOnly>{children}</SignedOutOnly>
      </Suspense>
    </SessionProvider>
  );
}
