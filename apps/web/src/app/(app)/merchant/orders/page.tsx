import type { Metadata } from 'next';
import { RequireRole } from '@/components/layout/require-role';
import { MerchantOrdersView } from '@/components/merchant/orders-view';

export const metadata: Metadata = { title: 'Orders — SwiftDrop' };

export default function Page() {
  return <RequireRole roles={['MERCHANT']} audience="merchants"><MerchantOrdersView /></RequireRole>;
}
