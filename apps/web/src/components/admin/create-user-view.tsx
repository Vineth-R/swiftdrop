'use client';

import { PageHeader } from '../layout/page-header';
import { Section } from '../layout/section';
import { CreateUserForm } from './create-user-form';

/** /admin/users/new: an administrator creates a courier, dispatcher or administrator. */
export function CreateUserView() {
  return (
    <>
      <PageHeader
        title="Create user"
        description="Add a courier, dispatcher or administrator. Customers and merchants register themselves."
      />
      <Section
        title="Account details"
        description="They get an email that the account exists. Tell them the password yourself."
      >
        <CreateUserForm />
      </Section>
    </>
  );
}
