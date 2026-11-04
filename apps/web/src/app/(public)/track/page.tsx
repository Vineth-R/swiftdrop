import type { Metadata } from 'next';

import { PageHeader } from '../../../components/layout/page-header';

export const metadata: Metadata = { title: 'Track a parcel — SwiftDrop' };

/**
 * A stand-in that shows where a public page goes: inside `app/(public)/`, which
 * gives it the public frame and no login check. Replace the body with the real
 * tracking page (API 4.6) built from `WaybillStub isPublic` and `Timeline`.
 */
export default function TrackPage() {
  return (
    <PageHeader
      title="Track a parcel"
      description="Enter a tracking number to see where a parcel is. This page is not built yet."
    />
  );
}
