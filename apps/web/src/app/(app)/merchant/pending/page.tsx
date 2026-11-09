import type { Metadata } from 'next';
import { RequireRole } from '@/components/layout/require-role';
import { MerchantStatusView } from '@/components/merchant/status-view';
export const metadata: Metadata = { title: 'Merchant approval — SwiftDrop' };
export default function Page() {
  return <RequireRole roles={['MERCHANT']} audience="merchants"><MerchantStatusView /></RequireRole>;
}
