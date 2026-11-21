import type { Metadata } from 'next';

import { SecurityView } from '../../../../components/profile/security-view';

export const metadata: Metadata = { title: 'Security — SwiftDrop' };

export default function SecurityPage() {
  return <SecurityView />;
}
