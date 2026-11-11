'use client';

import { getMerchant, listMerchantKeys } from '@/lib/merchant-api';
import { useLoad } from '@/lib/use-load';
import { PageHeader } from '../layout/page-header';
import { Section } from '../layout/section';
import { ButtonLink } from '../ui/button';
import { EmptyState } from '../ui/empty-state';
import { KeyValueList } from '../ui/key-value-list';
import { Skeleton, SkeletonGroup } from '../ui/skeleton';
import { StatusChip } from '../ui/status-chip';
import { LoadError } from './load-error';

/**
 * A merchant's home: the business as SwiftDrop has it, and whether an API key
 * exists yet. Orders are not here because the orders API is not built; this
 * screen shows only what the API can say today.
 */
export function MerchantDashboard({ name }: { name: string }) {
  const business = useLoad(getMerchant);
  const keys = useLoad(listMerchantKeys);

  const activeKeys =
    keys.state.status === 'ready'
      ? keys.state.data.filter((key) => key.status === 'ACTIVE').length
      : 0;

  return (
    <>
      <PageHeader
        title="Dashboard"
        description={`Signed in as ${name}.`}
        actions={
          <>
            <ButtonLink href="/api-keys">API keys</ButtonLink>
            <ButtonLink href="/merchant/profile" variant="secondary">
              Business profile
            </ButtonLink>
          </>
        }
      />

      <Section title="Your business">
        {business.state.status === 'error' ? (
          <LoadError
            title="Could not load your business"
            error={business.state.error}
            onRetry={business.retry}
          />
        ) : business.state.status === 'loading' ? (
          <SkeletonGroup label="Loading your business">
            <Skeleton className="h-5 w-1/3" />
            <Skeleton className="h-5 w-1/2" />
            <Skeleton className="h-5 w-1/4" />
          </SkeletonGroup>
        ) : (
          <KeyValueList
            layout="inline"
            items={[
              { label: 'Business', value: business.state.data.businessName },
              {
                label: 'Registration number',
                value: business.state.data.registrationNumber,
                isMono: true,
              },
              {
                label: 'Status',
                value: <StatusChip status={business.state.data.status} />,
              },
              {
                label: 'Pickup address',
                value: business.state.data.pickupAddress,
              },
            ]}
          />
        )}
      </Section>

      <Section
        title="API keys"
        description="Your systems use a key to create parcels on SwiftDrop."
      >
        {keys.state.status === 'error' ? (
          <LoadError
            title="Could not load your API keys"
            error={keys.state.error}
            onRetry={keys.retry}
          />
        ) : keys.state.status === 'loading' ? (
          <SkeletonGroup label="Loading your API keys">
            <Skeleton className="h-5 w-1/4" />
          </SkeletonGroup>
        ) : activeKeys === 0 ? (
          <EmptyState
            title="No API keys yet"
            action={<ButtonLink href="/api-keys">Create an API key</ButtonLink>}
          >
            Create a key to connect your shop or system to SwiftDrop.
          </EmptyState>
        ) : (
          <p className="text-body text-text-muted">
            You have{' '}
            <span className="font-mono text-text tabular">{activeKeys}</span>{' '}
            active {activeKeys === 1 ? 'key' : 'keys'}.{' '}
            <ButtonLink href="/api-keys" variant="secondary" size="sm">
              Manage API keys
            </ButtonLink>
          </p>
        )}
      </Section>
    </>
  );
}
