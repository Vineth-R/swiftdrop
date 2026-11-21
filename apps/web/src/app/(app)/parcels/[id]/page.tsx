import type { Metadata } from 'next';
import { ParcelDetailView } from '../../../../components/parcels/parcel-detail-view';

export const metadata: Metadata = { title: 'Parcel details — SwiftDrop' };

export default async function ParcelDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <ParcelDetailView id={id} />;
}
