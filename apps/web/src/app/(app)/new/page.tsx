import type { Metadata } from 'next';
import { NewDeliveryView } from '../../../components/parcels/new-delivery-view';

export const metadata: Metadata = { title: 'New delivery — SwiftDrop' };

export default function NewDeliveryPage() {
  return <NewDeliveryView />;
}
