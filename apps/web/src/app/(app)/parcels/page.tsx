'use client';

import { DispatcherParcelsView } from '../../../components/dispatch/dispatcher-parcels-view';
import { SystemPage } from '../../../components/layout/system-page';
import { ButtonLink } from '../../../components/ui/button';
import { roleSatisfies } from '../../../lib/roles';
import { useSession } from '../../../lib/session';

export default function ParcelsPage() {
  const { session } = useSession();

  if (session.status !== 'authenticated') {
    return null;
  }

  const { role } = session.user;

  if (roleSatisfies(role, 'DISPATCHER')) {
    return <DispatcherParcelsView />;
  }

  if (role === 'CUSTOMER') {
    return (
      <SystemPage
        label="404 · Not found"
        title="This page does not exist"
        actions={<ButtonLink href="/dashboard">Go to my dashboard</ButtonLink>}
      >
        The link may be out of date, or the screen may not be built yet.
      </SystemPage>
    );
  }

  return (
    <SystemPage
      label="403 · Forbidden"
      title="You do not have access"
      actions={
        <ButtonLink href="/dashboard" variant="secondary">
          Back to my dashboard
        </ButtonLink>
      }
    >
      This page is for dispatchers, administrators, and customers.
    </SystemPage>
  );
}

