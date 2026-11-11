'use client';

import { listAdminUsers } from '@/lib/admin-users-api';
import { listUnassignedParcels } from '@/lib/dispatch-api';
import { listAdminMerchants } from '@/lib/merchant-api';
import { useLoad, type Load } from '@/lib/use-load';
import { PageHeader } from '../layout/page-header';
import { Section } from '../layout/section';
import { ButtonLink } from '../ui/button';
import { StatCard } from './stat-card';

/** A list call that asked for one row, only to read how many there are in all. */
function totalOf(result: {
  data: unknown[];
  meta: { pagination?: { totalItems: number } };
}) {
  return result.meta.pagination?.totalItems ?? result.data.length;
}

/** Turns the state of a one-row list call into the state of its count. */
function counted(
  state: Load<{
    data: unknown[];
    meta: { pagination?: { totalItems: number } };
  }>,
): Load<number> {
  return state.status === 'ready'
    ? { status: 'ready', data: totalOf(state.data) }
    : state;
}

/**
 * An administrator's home: the three things most likely to be waiting on them,
 * each from its own call so one that fails does not hide the others, and the
 * places they manage.
 */
export function AdminDashboard({ name }: { name: string }) {
  const applications = useLoad((signal) =>
    listAdminMerchants({ status: 'PENDING', page: 1, pageSize: 1 }, signal),
  );
  const users = useLoad((signal) =>
    listAdminUsers({ page: 1, pageSize: 1 }, signal),
  );
  const unassigned = useLoad((signal) =>
    listUnassignedParcels({ page: 1, pageSize: 1 }, signal),
  );

  return (
    <>
      <PageHeader
        title="Dashboard"
        description={`Signed in as ${name}.`}
        actions={
          <ButtonLink href="/admin/merchants">Review merchants</ButtonLink>
        }
      />

      <Section title="Waiting for you">
        <div className="grid gap-4 md:grid-cols-3">
          <StatCard
            label="Merchant applications"
            state={counted(applications.state)}
            onRetry={applications.retry}
            href="/admin/merchants"
            linkLabel="Review applications"
            zeroText="No applications are waiting."
          />
          <StatCard
            label="Unassigned parcels"
            state={counted(unassigned.state)}
            onRetry={unassigned.retry}
            href="/unassigned"
            linkLabel="Assign couriers"
            zeroText="Every parcel has a courier."
          />
          <StatCard
            label="Accounts"
            state={counted(users.state)}
            onRetry={users.retry}
            href="/admin/users"
            linkLabel="Manage accounts"
            zeroText="There are no accounts."
          />
        </div>
      </Section>

      <Section title="Manage">
        <div className="flex flex-wrap gap-3">
          <ButtonLink href="/admin/users" variant="secondary">
            Users
          </ButtonLink>
          <ButtonLink href="/admin/merchants" variant="secondary">
            Merchants
          </ButtonLink>
          <ButtonLink href="/admin/zones" variant="secondary">
            Zones
          </ButtonLink>
          <ButtonLink href="/admin/pricing" variant="secondary">
            Pricing
          </ButtonLink>
        </div>
      </Section>
    </>
  );
}
