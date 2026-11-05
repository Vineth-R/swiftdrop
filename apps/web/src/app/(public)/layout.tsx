import type { ReactNode } from 'react';

import { PublicLayout } from '../../components/layout/public-layout';

/** Every page in this group is public: no login, no sidebar, public data only. */
export default function PublicGroupLayout({
  children,
}: {
  children: ReactNode;
}) {
  // The tracking board is two columns on a laptop, so the column is wide.
  return <PublicLayout width="wide">{children}</PublicLayout>;
}
