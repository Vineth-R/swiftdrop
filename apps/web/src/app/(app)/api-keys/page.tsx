import type { Metadata } from 'next';

import { MerchantKeysView } from '../../../components/merchant/keys-view';
import { RequireRole } from '../../../components/layout/require-role';

export const metadata: Metadata = { title: 'API keys — SwiftDrop' };

export default function ApiKeysPage() {
  return (
    <RequireRole roles={['MERCHANT']} audience="approved merchants">
      <MerchantKeysView />
    </RequireRole>
  );
}
