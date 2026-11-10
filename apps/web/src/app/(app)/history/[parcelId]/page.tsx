import type { Metadata } from 'next';
import { DeliveryDetailView } from '../../../../components/courier/delivery-detail-view';

export const metadata: Metadata = { title: 'Delivery details — SwiftDrop' };
export default async function Page({
  params,
}: {
  params: Promise<{ parcelId: string }>;
}) {
  const { parcelId } = await params;
  return <DeliveryDetailView parcelId={parcelId} />;
}
