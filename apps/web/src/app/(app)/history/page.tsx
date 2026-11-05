import type { Metadata } from 'next';

import { HistoryView } from '../../../components/courier/history-view';

export const metadata: Metadata = { title: 'History — SwiftDrop' };

export default function HistoryPage() {
  return <HistoryView />;
}
