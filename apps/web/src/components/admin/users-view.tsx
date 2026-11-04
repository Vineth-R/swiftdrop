'use client';

import { Lock } from 'lucide-react';
import { useEffect, useState, type FormEvent } from 'react';

import {
  ADMIN_USERS_PAGE_SIZE,
  listAdminUsers,
  setUserActive,
  type AdminUser,
} from '../../lib/admin-users-api';
import { ApiError, type ApiMeta } from '../../lib/api';
import { useSession } from '../../lib/session';
import { PageHeader } from '../layout/page-header';
import { Section } from '../layout/section';
import { Alert } from '../ui/alert';
import { Button } from '../ui/button';
import { Spinner } from '../ui/spinner';
import { TextField } from '../ui/field';
import { UserList } from './user-list';

type Pagination = NonNullable<ApiMeta['pagination']>;

type Loaded = { key: string } & (
  | { kind: 'ok'; users: AdminUser[]; pagination: Pagination }
  | { kind: 'error'; error: ApiError }
);

function asApiError(error: unknown): ApiError {
  return error instanceof ApiError
    ? error
    : new ApiError({ code: 'INTERNAL', status: 0 });
}

/** Why a list or an action failed, in words that never repeat the server's. */
function FailureAlert({
  error,
  onRetry,
}: {
  error: ApiError;
  onRetry?: () => void;
}) {
  const retry = onRetry && (
    <Button variant="secondary" size="sm" onClick={onRetry}>
      Try again
    </Button>
  );
  if (error.code === 'FORBIDDEN') {
    return (
      <Alert tone="warning" icon={Lock} title="You do not have access to this">
        Only administrators can manage users. If you need access, ask an
        administrator.
      </Alert>
    );
  }
  if (error.code === 'RATE_LIMITED') {
    return (
      <Alert tone="warning" title="Too many requests" actions={retry}>
        {error.retryAfter
          ? `Wait ${error.retryAfter} seconds and try again.`
          : 'Wait a moment and try again.'}
      </Alert>
    );
  }
  if (error.code === 'CANNOT_DEACTIVATE_SELF') {
    return (
      <Alert tone="warning" title="You cannot deactivate your own account">
        Ask another administrator to do it if it is needed.
      </Alert>
    );
  }
  if (error.code === 'CONFLICT') {
    return (
      <Alert tone="warning" title="This account was changed by someone else" actions={retry}>
        Reload the list to see how it looks now.
      </Alert>
    );
  }
  if (error.code === 'NOT_FOUND') {
    return (
      <Alert tone="warning" title="This account no longer exists" actions={retry}>
        Reload the list to see how it looks now.
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

/** /admin/users: every account, searchable, with activate and deactivate. */
export function UsersView() {
  const { session } = useSession();
  const currentUserId =
    session.status === 'authenticated' ? session.user.id : '';

  const [draft, setDraft] = useState('');
  const [query, setQuery] = useState('');
  const [page, setPage] = useState(1);
  const [reload, setReload] = useState(0);
  const [loaded, setLoaded] = useState<Loaded | null>(null);
  const [pendingId, setPendingId] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [actionError, setActionError] = useState<ApiError | null>(null);

  const key = `${query}|${page}|${reload}`;

  useEffect(() => {
    const controller = new AbortController();
    listAdminUsers(
      {
        q: query || undefined,
        page,
        pageSize: ADMIN_USERS_PAGE_SIZE,
      },
      controller.signal,
    )
      .then(({ data, meta }) => {
        setLoaded({
          key,
          kind: 'ok',
          users: data,
          pagination: meta.pagination ?? {
            page,
            pageSize: ADMIN_USERS_PAGE_SIZE,
            totalItems: data.length,
            totalPages: 1,
          },
        });
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        setLoaded({ key, kind: 'error', error: asApiError(error) });
      });
    return () => controller.abort();
  }, [key, query, page]);

  const current = loaded && loaded.key === key ? loaded : null;

  function search(event: FormEvent) {
    event.preventDefault();
    setNotice(null);
    setActionError(null);
    setPage(1);
    setQuery(draft.trim());
  }

  function clearSearch() {
    setDraft('');
    setNotice(null);
    setActionError(null);
    setPage(1);
    setQuery('');
  }

  function goToPage(next: number) {
    setNotice(null);
    setActionError(null);
    setPage(next);
  }

  async function changeActive(user: AdminUser, isActive: boolean) {
    setPendingId(user.id);
    setNotice(null);
    setActionError(null);
    try {
      await setUserActive(user.id, isActive);
      setLoaded((previous) =>
        previous && previous.kind === 'ok'
          ? {
              ...previous,
              users: previous.users.map((row) =>
                row.id === user.id ? { ...row, isActive } : row,
              ),
            }
          : previous,
      );
      setNotice(
        isActive
          ? `${user.fullName} can sign in again.`
          : `${user.fullName} is deactivated and signed out of every device.`,
      );
    } catch (error) {
      setActionError(asApiError(error));
    } finally {
      setPendingId(null);
    }
  }

  const ready = current?.kind === 'ok' ? current : null;

  return (
    <>
      <PageHeader
        title="Users"
        description="Accounts for customers, merchants, couriers, dispatchers and administrators."
      />
      <Section
        title="All users"
        description="Search by name or email. A deactivated user cannot sign in."
      >
        <form
          onSubmit={search}
          role="search"
          className="flex flex-wrap items-end gap-3"
        >
          <TextField
            id="user-search"
            label="Search users"
            type="search"
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            placeholder="Name or email"
            className="min-w-0 flex-1"
          />
          <Button type="submit">Search</Button>
        </form>

        {notice && <Alert tone="success" title={notice} />}
        {actionError && (
          <FailureAlert
            error={actionError}
            onRetry={
              actionError.code === 'CONFLICT' || actionError.code === 'NOT_FOUND'
                ? () => {
                    setActionError(null);
                    setReload((value) => value + 1);
                  }
                : undefined
            }
          />
        )}

        {current === null && (
          <p className="flex items-center gap-3 text-body-sm text-text-muted">
            <Spinner tone="standalone" label="Loading users" />
            Loading users
          </p>
        )}

        {current?.kind === 'error' && (
          <FailureAlert
            error={current.error}
            onRetry={() => setReload((value) => value + 1)}
          />
        )}

        {ready && ready.users.length === 0 && (
          <div className="flex flex-col items-start gap-3 rounded-card border px-4 py-6">
            {/* TODO(design-foundation): replace with EmptyState. */}
            <p className="text-body text-text">
              {query ? `No users match “${query}”.` : 'There are no users yet.'}
            </p>
            {query && (
              <Button variant="secondary" size="sm" onClick={clearSearch}>
                Clear search
              </Button>
            )}
          </div>
        )}

        {ready && ready.users.length > 0 && (
          <>
            <UserList
              users={ready.users}
              currentUserId={currentUserId}
              pendingId={pendingId}
              onSetActive={changeActive}
            />
            <nav
              aria-label="Pagination"
              className="flex flex-wrap items-center justify-between gap-3"
            >
              <p className="text-body-sm text-text-muted">
                Page {ready.pagination.page} of{' '}
                {Math.max(ready.pagination.totalPages, 1)} ·{' '}
                {ready.pagination.totalItems}{' '}
                {ready.pagination.totalItems === 1 ? 'user' : 'users'}
              </p>
              <div className="flex gap-3">
                <Button
                  variant="secondary"
                  size="sm"
                  isDisabled={ready.pagination.page <= 1}
                  onClick={() => goToPage(ready.pagination.page - 1)}
                >
                  Previous
                </Button>
                <Button
                  variant="secondary"
                  size="sm"
                  isDisabled={
                    ready.pagination.page >= ready.pagination.totalPages
                  }
                  onClick={() => goToPage(ready.pagination.page + 1)}
                >
                  Next
                </Button>
              </div>
            </nav>
          </>
        )}
      </Section>
    </>
  );
}
