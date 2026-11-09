import type { Metadata } from 'next';

import { CourierDeliveryDetailView } from '../../../../components/courier/delivery-detail-view';

export const metadata: Metadata = { title: 'Delivery — SwiftDrop' };

export default async function CourierDeliveryDetailPage({
  params,
}: {
  params: Promise<{ parcelId: string }>;
}) {
  const { parcelId } = await params;
  return <CourierDeliveryDetailView parcelId={parcelId} />;
}
