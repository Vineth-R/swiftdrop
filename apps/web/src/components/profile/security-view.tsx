'use client';

import { useSession } from '../../lib/session';
import { PageHeader } from '../layout/page-header';
import { Section } from '../layout/section';
import { ButtonLink } from '../ui/button';
import { PasswordForm } from './password-form';

/** /profile/security: change the password. Every role sees it. */
export function SecurityView() {
  const { session } = useSession();
  if (session.status !== 'authenticated') return null;

  return (
    <>
      <PageHeader
        title="Security"
        description="Change the password you use to log in."
        actions={
          <ButtonLink href="/profile" variant="secondary">
            Back to profile
          </ButtonLink>
        }
      />
      <Section title="Change password">
        <PasswordForm user={session.user} />
      </Section>
    </>
  );
}
