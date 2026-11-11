import type { Metadata } from 'next';

import { RequireRole } from '@/components/layout/require-role';
import { WebhookView } from '@/components/merchant/webhook-view';

export const metadata: Metadata = { title: 'Webhook — SwiftDrop' };

export default function WebhookPage() {
  return (
    <RequireRole roles={['MERCHANT']} audience="approved merchants">
      <WebhookView />
    </RequireRole>
  );
}
