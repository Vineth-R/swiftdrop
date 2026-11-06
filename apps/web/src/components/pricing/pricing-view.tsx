'use client';

import { PageHeader } from '../layout/page-header';
import { PricingRulesSection } from './pricing-rules-section';
import { SizeSurchargesSection } from './size-surcharges-section';
import { WeightBandsSection } from './weight-bands-section';

/** /admin/pricing: everything that makes up a delivery fee. */
export function PricingView() {
  return (
    <>
      <PageHeader
        title="Pricing"
        description="The base fees, weight charges and size surcharges that make up a delivery fee."
      />
      {/* TODO(design-foundation): needs Tabs */}
      <PricingRulesSection />
      <WeightBandsSection />
      <SizeSurchargesSection />
    </>
  );
}
