import type { Metadata } from 'next';

import { TrackingView } from '../../../components/tracking/tracking-view';

export const metadata: Metadata = { title: 'Track a parcel — SwiftDrop' };

export default function TrackPage() {
  return <TrackingView />;
}
