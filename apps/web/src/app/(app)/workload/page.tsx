import type { Metadata } from 'next';

import { WorkloadView } from '../../../components/dispatch/workload-view';

export const metadata: Metadata = { title: 'Courier Workload — SwiftDrop' };

export default function WorkloadPage() {
  return <WorkloadView />;
}
