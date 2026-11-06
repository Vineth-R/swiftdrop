import type { Metadata } from 'next';

import { UnassignedView } from '../../../components/dispatch/unassigned-view';

export const metadata: Metadata = { title: 'Unassigned Parcels — SwiftDrop' };

export default function UnassignedPage() {
  return <UnassignedView />;
}
