'use client';

import { useSession } from '../../lib/session';
import { PageHeader } from '../layout/page-header';
import { Section } from '../layout/section';

/**
 * Stands in for every role's dashboard until it exists. It proves the sign-in,
 * the role and the shell work end to end. Replace the page that renders it with
 * the real dashboard; the shell and the session gate are in app/(app)/layout.tsx.
 */
export function DashboardPlaceholder() {
  const { session } = useSession();
  if (session.status !== 'authenticated') return null;
  const { user } = session;

  return (
    <>
      <PageHeader
        title="Dashboard"
        description="This is a placeholder until each role gets its own dashboard."
      />
      <Section title="Who is signed in">
        <p className="text-body text-text-muted">
          Signed in as{' '}
          <span className="font-semibold text-text">{user.fullName}</span>{' '}
          <span className="font-mono text-body-sm">({user.email})</span> with
          the role <span className="font-mono text-body-sm">{user.role}</span>.
        </p>
      </Section>
    </>
  );
}
