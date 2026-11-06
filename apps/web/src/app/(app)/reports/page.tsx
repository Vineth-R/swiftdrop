import type { Metadata } from 'next';

import { ReportsView } from '../../../components/reports/reports-view';

export const metadata: Metadata = { title: 'Reports — SwiftDrop' };

export default function ReportsPage() {
  return <ReportsView />;
}