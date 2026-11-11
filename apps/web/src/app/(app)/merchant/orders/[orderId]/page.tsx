import type { Metadata } from 'next';

import { RequireRole } from '../../../../../components/layout/require-role';
import { MerchantOrderDetailView } from '../../../../../components/merchant/order-detail-view';

export const metadata: Metadata = { title: 'Order details — SwiftDrop' };

export default async function MerchantOrderDetailPage({
  params,
}: {
  params: Promise<{ orderId: string }>;
}) {
  const { orderId } = await params;
  return (
    <RequireRole roles={['MERCHANT']} audience="merchants">
      <MerchantOrderDetailView orderId={orderId} />
    </RequireRole>
  );
}
