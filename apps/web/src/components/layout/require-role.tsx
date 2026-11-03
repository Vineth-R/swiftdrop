'use client';

import type { ReactNode } from 'react';

import { describeRole, roleSatisfies } from '../../lib/roles';
import { useSession } from '../../lib/session';
import type { Role } from '../../lib/tokens';
import { ButtonLink } from '../ui/button';
import { SystemPage } from './system-page';

type RequireRoleProps = {
  /** Any one of these roles may see the page. An ADMIN also passes for DISPATCHER. */
  roles: Role[];
  /** The people the page is for, as a plural noun phrase: "administrators". */
  audience: string;
  children: ReactNode;
};

/**
 * Wrap a page's content in this when only some roles may open it. Someone with
 * another role sees the 403 page inside the shell, not an empty page. Hiding a
 * link in the sidebar is never enough: the page itself has to check, and the
 * API checks again.
 */
export function RequireRole({ roles, audience, children }: RequireRoleProps) {
  const { session } = useSession();
  if (session.status !== 'authenticated') return null;

  const { role } = session.user;
  if (roles.some((required) => roleSatisfies(role, required))) {
    return <>{children}</>;
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
      This page is for {audience}. You are signed in as {describeRole(role)},
      which is probably correct: nothing is wrong with your account.
    </SystemPage>
  );
}
