import type { Metadata } from 'next';
import { Suspense } from 'react';

import { PageHeader } from '../../../components/layout/page-header';
import { TrackView } from '../../../components/tracking/track-view';

export const metadata: Metadata = { title: 'Track a parcel — SwiftDrop' };

export default function TrackPage() {
  return (
    <Suspense
      fallback={
        <PageHeader
          title="Track a parcel"
          description="Enter a tracking number to see where a parcel is."
        />
      }
    >
      <TrackView />
    </Suspense>
  );
}
