'use client';

import { useSession } from '../../lib/session';
import { PageHeader } from '../layout/page-header';
import { Section } from '../layout/section';
import { ButtonLink } from '../ui/button';
import { AccountSummary } from './account-summary';
import { ProfileForm } from './profile-form';

/** /profile: who the person is, and the details they may change. Every role sees it. */
export function ProfileView() {
  const { session } = useSession();
  if (session.status !== 'authenticated') return null;
  const { user } = session;

  return (
    <>
      <PageHeader
        title="Profile"
        description="Your account details and how people reach you."
      />
      <Section title="Account">
        <AccountSummary user={user} />
      </Section>
      <Section
        title="Personal details"
        description="Change your name or phone number."
      >
        <ProfileForm user={user} />
      </Section>
      <Section
        title="Password"
        description="Keep your account safe with a password only you know."
      >
        <div>
          <ButtonLink href="/profile/security" variant="secondary">
            Change password
          </ButtonLink>
        </div>
      </Section>
    </>
  );
}
