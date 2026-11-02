import type { ReactNode } from 'react';

import { SignedInOnly } from '../../components/auth/session-gates';
import { SessionProvider } from '../../lib/session';

export default function AppGroupLayout({ children }: { children: ReactNode }) {
  return (
    <SessionProvider>
      <SignedInOnly>{children}</SignedInOnly>
    </SessionProvider>
  );
}
