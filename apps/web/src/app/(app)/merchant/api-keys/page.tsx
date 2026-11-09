import type { Metadata } from 'next';
import { RequireRole } from '@/components/layout/require-role';
import { MerchantKeysView } from '@/components/merchant/keys-view';

export const metadata: Metadata = { title: 'API keys — SwiftDrop' };

export default function Page() {
  return <RequireRole roles={['MERCHANT']} audience="merchants"><MerchantKeysView /></RequireRole>;
}
