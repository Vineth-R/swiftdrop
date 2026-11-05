import type { Metadata } from 'next';

import { TrackView } from '../../../components/tracking/track-view';

export const metadata: Metadata = { title: 'Track a parcel — SwiftDrop' };

export default function TrackPage() {
  return <TrackView />;
}
