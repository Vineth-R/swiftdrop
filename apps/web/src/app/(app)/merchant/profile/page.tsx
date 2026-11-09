import type { Metadata } from 'next';
import { RequireRole } from '@/components/layout/require-role';
import { BusinessProfileView } from '@/components/merchant/profile-view';

export const metadata: Metadata = { title: 'Business profile — SwiftDrop' };

export default function Page() {
  return <RequireRole roles={['MERCHANT']} audience="merchants"><BusinessProfileView /></RequireRole>;
}
