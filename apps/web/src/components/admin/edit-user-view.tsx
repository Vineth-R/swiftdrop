'use client';

import { Lock } from 'lucide-react';
import { useEffect, useState } from 'react';

import { getAdminUser, type AdminUserDetail } from '../../lib/admin-users-api';
import { ApiError } from '../../lib/api';
import { PageHeader } from '../layout/page-header';
import { Section } from '../layout/section';
import { Alert } from '../ui/alert';
import { Button, ButtonLink } from '../ui/button';
import { Skeleton, SkeletonGroup } from '../ui/skeleton';
import { EditUserForm } from './edit-user-form';

type State =
  | { status: 'loading' }
  | { status: 'ready'; user: AdminUserDetail }
  | { status: 'error'; error: ApiError };

function asApiError(error: unknown): ApiError {
  return error instanceof ApiError
    ? error
    : new ApiError({ code: 'INTERNAL', status: 0 });
}

/** Why the user could not be loaded, in words that never repeat the server's. */
function LoadFailure({
  error,
  onRetry,
}: {
  error: ApiError;
  onRetry: () => void;
}) {
  const back = (
    <ButtonLink href="/admin/users" variant="secondary" size="sm">
      Back to users
    </ButtonLink>
  );
  if (error.code === 'NOT_FOUND') {
    return (
      <Alert tone="warning" title="This account does not exist" actions={back}>
        It may have been removed, or the link is wrong.
      </Alert>
    );
  }
  if (error.code === 'FORBIDDEN') {
    return (
      <Alert tone="warning" icon={Lock} title="You do not have access to this">
        Only administrators can edit users. If you need access, ask an
        administrator.
      </Alert>
    );
  }
  const retry = (
    <Button variant="secondary" size="sm" onClick={onRetry}>
      Try again
    </Button>
  );
  if (error.code === 'RATE_LIMITED') {
    return (
      <Alert tone="warning" title="Too many requests" actions={retry}>
        {error.retryAfter
          ? `Wait ${error.retryAfter} seconds and try again.`
          : 'Wait a moment and try again.'}
      </Alert>
    );
  }
  return (
    <Alert
      tone="danger"
      title={
        error.code === 'NETWORK'
          ? 'We could not reach SwiftDrop'
          : 'Something broke on our side'
      }
      actions={retry}
    >
      {error.code === 'NETWORK'
        ? 'Check your connection and try again.'
        : 'Please try again in a moment.'}
      {error.requestId && error.isServerFault && (
        <span className="mt-2 block select-all font-mono text-overline text-text-faint">
          Reference {error.requestId}
        </span>
      )}
    </Alert>
  );
}

/** /admin/users/{id}/edit: loads one user from the API and offers the edit form. */
export function EditUserView({ userId }: { userId: string }) {
  const [state, setState] = useState<State>({ status: 'loading' });
  const [reload, setReload] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    setState({ status: 'loading' });
    getAdminUser(userId, controller.signal)
      .then((user) => setState({ status: 'ready', user }))
      .catch((error: unknown) => {
        if (!controller.signal.aborted) {
          setState({ status: 'error', error: asApiError(error) });
        }
      });
    return () => controller.abort();
  }, [userId, reload]);

  return (
    <>
      <PageHeader
        title="Edit user"
        description="Change a name, phone number, role or courier details. The email and password are not changed here."
      />
      <Section
        title="Account details"
        description={
          state.status === 'ready'
            ? `Editing ${state.user.fullName}.`
            : undefined
        }
      >
        {state.status === 'loading' && (
          <SkeletonGroup
            label="Loading user"
            className="flex max-w-xl flex-col gap-5"
          >
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-24 w-full" />
          </SkeletonGroup>
        )}
        {state.status === 'error' && (
          <LoadFailure
            error={state.error}
            onRetry={() => setReload((value) => value + 1)}
          />
        )}
        {state.status === 'ready' && <EditUserForm user={state.user} />}
      </Section>
    </>
  );
}
