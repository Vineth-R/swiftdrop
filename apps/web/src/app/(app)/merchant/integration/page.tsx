import type { Metadata } from 'next';

import { CreateViaAPI } from '../../../../components/merchant/create-via-api';
import { PageHeader } from '../../../../components/layout/page-header';
import { RequireRole } from '../../../../components/layout/require-role';

export const metadata: Metadata = { title: 'API integration — SwiftDrop' };

/** Merchant-only server-to-server integration onboarding and API guide. */
export default function MerchantIntegrationPage() {
  return (
    <RequireRole roles={['MERCHANT']} audience="merchants">
      <PageHeader
        title="API integration"
        description="Connect your store to SwiftDrop with an API key, create orders server-to-server and receive signed status webhooks."
      />
      <CreateViaAPI />
    </RequireRole>
  );
}
