import type { Metadata } from 'next';
import { ParcelsView } from '../../../components/parcels/parcels-view';

export const metadata: Metadata = { title: 'Parcels — SwiftDrop' };

export default function ParcelsPage() {
  return <ParcelsView />;
}
