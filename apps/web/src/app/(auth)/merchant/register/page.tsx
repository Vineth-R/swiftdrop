import type { Metadata } from 'next';
import { RegisterPanel } from '@/components/auth/panels';
import { AuthHeading, AuthLayout } from '@/components/layout/auth-layout';
import { MerchantRegisterForm } from '@/components/merchant/register-form';

export const metadata: Metadata = { title: 'Register your business — SwiftDrop' };
export default function Page() {
  return <AuthLayout panel={<RegisterPanel />} width="wide"><AuthHeading eyebrow="Merchant account" title="Deliver with SwiftDrop">Connect your store, manage delivery orders and keep customers informed.</AuthHeading><MerchantRegisterForm /></AuthLayout>;
}
