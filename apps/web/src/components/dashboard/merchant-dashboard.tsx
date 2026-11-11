'use client';

import Link from 'next/link';
import { Boxes } from 'lucide-react';

import {
  getMerchant,
  getMerchantOrderOverview,
  listMerchantKeys,
  type MerchantOrderOverview,
} from '@/lib/merchant-api';
import { useLoad, type Load } from '@/lib/use-load';
import { PageHeader } from '../layout/page-header';
import { Section } from '../layout/section';
import { ApprovalNotice } from '../merchant/shared';
import { money } from '../merchant/orders-view';
import { ButtonLink } from '../ui/button';
import { EmptyState } from '../ui/empty-state';
import { KeyValueList } from '../ui/key-value-list';
import { Skeleton, SkeletonGroup } from '../ui/skeleton';
import { StatusChip } from '../ui/status-chip';
import { LoadError } from './load-error';
import { StatCard } from './stat-card';

function metric(
  state: Load<MerchantOrderOverview>,
  pick: (overview: MerchantOrderOverview) => number,
): Load<number> {
  if (state.status !== 'ready') return state;
  return { status: 'ready', data: pick(state.data) };
}

/**
 * A merchant's home: the business as SwiftDrop has it, a real order overview,
 * and whether an API key exists yet. Every number comes from the merchant order
 * API; nothing is hardcoded.
 */
export function MerchantDashboard({ name }: { name: string }) {
  const business = useLoad(getMerchant);
  const keys = useLoad(listMerchantKeys);
  const orders = useLoad(getMerchantOrderOverview);

  const approved =
    business.state.status === 'ready' &&
    business.state.data.status === 'APPROVED';
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
            <ButtonLink href="/merchant/orders">Orders</ButtonLink>
            <ButtonLink href="/api-keys" variant="secondary">
              API keys
            </ButtonLink>
            <ButtonLink href="/merchant/profile" variant="secondary">
              Business profile
            </ButtonLink>
          </>
        }
      />

      {business.state.status === 'ready' ? (
        <ApprovalNotice merchant={business.state.data} />
      ) : null}

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

      {approved ? (
        <Section
          title="Orders"
          description="Orders created by your store, updated as parcels move."
        >
          {orders.state.status === 'error' ? (
            <LoadError
              title="Could not load your orders"
              error={orders.state.error}
              onRetry={orders.retry}
            />
          ) : (
            <>
              <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
                <StatCard
                  label="Total orders"
                  state={metric(orders.state, (overview) => overview.total)}
                  onRetry={orders.retry}
                  href="/merchant/orders"
                  linkLabel="View all orders"
                  zeroText="No orders yet."
                />
                <StatCard
                  label="In progress"
                  state={metric(
                    orders.state,
                    (overview) => overview.inProgress,
                  )}
                  onRetry={orders.retry}
                  href="/merchant/orders"
                  linkLabel="See orders in progress"
                  zeroText="Nothing is in progress."
                />
                <StatCard
                  label="Delivered"
                  state={metric(orders.state, (overview) => overview.delivered)}
                  onRetry={orders.retry}
                  href="/merchant/orders"
                  linkLabel="See delivered orders"
                  zeroText="No delivered orders yet."
                />
                <StatCard
                  label="Failed"
                  state={metric(orders.state, (overview) => overview.failed)}
                  onRetry={orders.retry}
                  href="/merchant/orders"
                  linkLabel="See failed orders"
                  zeroText="No failed orders."
                />
              </div>

              <div className="mt-6">
                {orders.state.status === 'loading' ? (
                  <SkeletonGroup label="Loading recent orders">
                    <Skeleton className="h-16 w-full" />
                    <Skeleton className="h-16 w-full" />
                  </SkeletonGroup>
                ) : orders.state.data.recent.length === 0 ? (
                  <EmptyState
                    title="No orders yet"
                    icon={Boxes}
                    action={
                      <ButtonLink href="/merchant/orders">
                        Create via API
                      </ButtonLink>
                    }
                  >
                    Create an API key, then post an order from your store. The
                    orders page has a request example you can copy.
                  </EmptyState>
                ) : (
                  <ul className="divide-y divide-line overflow-hidden rounded-card border border-line bg-surface">
                    {orders.state.data.recent.map((order) => (
                      <li
                        key={order.orderId}
                        className="grid gap-3 p-5 md:grid-cols-[1.2fr_1fr_auto_auto] md:items-center"
                      >
                        <div className="min-w-0">
                          <Link
                            className="break-all text-label text-interactive underline underline-offset-4"
                            href={`/merchant/orders/${encodeURIComponent(order.orderId)}`}
                          >
                            {order.externalOrderId}
                          </Link>
                          <p className="mt-1 break-all font-mono text-caption text-text-muted">
                            {order.trackingNumber}
                          </p>
                        </div>
                        <p className="text-body-sm">
                          {order.recipientName}
                          <span className="block text-caption text-text-muted">
                            {order.dropoffCity}
                          </span>
                        </p>
                        <StatusChip status={order.status} />
                        <p className="font-mono text-body-sm tabular-nums md:text-right">
                          {money(order.total, order.currency)}
                        </p>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            </>
          )}
        </Section>
      ) : null}

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
