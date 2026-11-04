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
import { roles, type Role } from '../../lib/tokens';
import { PageHeader } from '../layout/page-header';
import { Section } from '../layout/section';
import { Alert } from '../ui/alert';
import { Button } from '../ui/button';
import { TablePagination } from '../ui/data-table';
import { EmptyState } from '../ui/empty-state';
import { TextField } from '../ui/field';
import { Modal } from '../ui/modal';
import { Select } from '../ui/select';
import { useToast } from '../ui/toast';
import { UserList } from './user-list';

type Pagination = NonNullable<ApiMeta['pagination']>;

type Loaded = { key: string } & (
  | { kind: 'ok'; users: AdminUser[]; pagination: Pagination }
  | { kind: 'error'; error: ApiError }
);

type StatusFilter = '' | 'ACTIVE' | 'DEACTIVATED';

/** The longest search the API accepts (API v1.1 section 2.10). */
const SEARCH_MAX_LENGTH = 100;

const roleOptions = [
  { value: '', label: 'All roles' },
  ...roles.map((role) => ({ value: role, label: role })),
];

const statusOptions = [
  { value: '', label: 'All accounts' },
  { value: 'ACTIVE', label: 'Active' },
  { value: 'DEACTIVATED', label: 'Deactivated' },
];

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
  if (error.code === 'VALIDATION_FAILED') {
    return (
      <Alert tone="warning" title="That search was not accepted">
        {error.issues.some((issue) => issue.field === 'q')
          ? `Use ${SEARCH_MAX_LENGTH} characters or fewer to search.`
          : 'Check the search and filters, then try again.'}
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
      <Alert
        tone="warning"
        title="This account was changed by someone else"
        actions={retry}
      >
        Reload the list to see how it looks now.
      </Alert>
    );
  }
  if (error.code === 'NOT_FOUND') {
    return (
      <Alert
        tone="warning"
        title="This account no longer exists"
        actions={retry}
      >
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

/** /admin/users: every account, searchable and filterable, with activate and deactivate. */
export function UsersView() {
  const { session } = useSession();
  const { show } = useToast();
  const currentUserId =
    session.status === 'authenticated' ? session.user.id : '';

  const [draft, setDraft] = useState('');
  const [query, setQuery] = useState('');
  const [role, setRole] = useState<Role | ''>('');
  const [status, setStatus] = useState<StatusFilter>('');
  const [page, setPage] = useState(1);
  const [reload, setReload] = useState(0);
  const [loaded, setLoaded] = useState<Loaded | null>(null);
  // The last pagination the API sent. It stays while the next page loads, so
  // the Previous and Next buttons do not vanish under the person's focus.
  const [lastPagination, setLastPagination] = useState<Pagination | null>(null);
  const [pendingId, setPendingId] = useState<string | null>(null);
  const [confirming, setConfirming] = useState<AdminUser | null>(null);
  const [actionError, setActionError] = useState<ApiError | null>(null);

  const key = `${query}|${role}|${status}|${page}|${reload}`;

  useEffect(() => {
    const controller = new AbortController();
    listAdminUsers(
      {
        q: query || undefined,
        role: role || undefined,
        isActive: status === '' ? undefined : status === 'ACTIVE',
        page,
        pageSize: ADMIN_USERS_PAGE_SIZE,
      },
      controller.signal,
    )
      .then(({ data, meta }) => {
        const pagination = meta.pagination ?? {
          page,
          pageSize: ADMIN_USERS_PAGE_SIZE,
          totalItems: data.length,
          totalPages: 1,
        };
        setLastPagination(pagination);
        // The number of pages can shrink while someone is on a later page
        // (a filter, or users removed elsewhere). Ask again for the last page
        // there is; no results at all means page 1.
        const lastPage = Math.max(pagination.totalPages, 1);
        if (page > lastPage) {
          setPage(lastPage);
          return;
        }
        setLoaded({ key, kind: 'ok', users: data, pagination });
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        setLoaded({ key, kind: 'error', error: asApiError(error) });
      });
    return () => controller.abort();
  }, [key, query, role, status, page]);

  const current = loaded && loaded.key === key ? loaded : null;
  const ready = current?.kind === 'ok' ? current : null;
  const hasFilters = query !== '' || role !== '' || status !== '';
  // The API refused the search text itself (a 422 on `q`): say so at the field.
  const searchError =
    current?.kind === 'error' &&
    current.error.code === 'VALIDATION_FAILED' &&
    current.error.issues.some((issue) => issue.field === 'q')
      ? `Use ${SEARCH_MAX_LENGTH} characters or fewer.`
      : undefined;

  function search(event: FormEvent) {
    event.preventDefault();
    setActionError(null);
    setLastPagination(null);
    setPage(1);
    setQuery(draft.trim());
    // Searching again is a request even when the words are the same, such as
    // after a failure.
    setReload((value) => value + 1);
  }

  function changeRole(value: string) {
    setActionError(null);
    setLastPagination(null);
    setPage(1);
    setRole(value as Role | '');
  }

  function changeStatus(value: string) {
    setActionError(null);
    setLastPagination(null);
    setPage(1);
    setStatus(value as StatusFilter);
  }

  function clearFilters() {
    setDraft('');
    setActionError(null);
    setLastPagination(null);
    setPage(1);
    setQuery('');
    setRole('');
    setStatus('');
  }

  function goToPage(next: number) {
    const lastPage = Math.max(lastPagination?.totalPages ?? 1, 1);
    setActionError(null);
    setPage(Math.min(Math.max(next, 1), lastPage));
  }

  async function changeActive(user: AdminUser, isActive: boolean) {
    setPendingId(user.id);
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
      show({
        title: isActive
          ? `${user.fullName} can sign in again.`
          : `${user.fullName} is deactivated and signed out of every device.`,
      });
    } catch (error) {
      setActionError(asApiError(error));
    } finally {
      setPendingId(null);
    }
  }

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
            maxLength={SEARCH_MAX_LENGTH}
            error={searchError}
            className="min-w-0 flex-1"
          />
          <Select
            id="user-role"
            label="Role"
            options={roleOptions}
            value={role}
            onChange={(event) => changeRole(event.target.value)}
          />
          <Select
            id="user-status"
            label="Account"
            options={statusOptions}
            value={status}
            onChange={(event) => changeStatus(event.target.value)}
          />
          <Button type="submit">Search</Button>
        </form>

        {actionError && (
          <FailureAlert
            error={actionError}
            onRetry={
              actionError.code === 'CONFLICT' ||
              actionError.code === 'NOT_FOUND'
                ? () => {
                    setActionError(null);
                    setReload((value) => value + 1);
                  }
                : undefined
            }
          />
        )}

        {current?.kind === 'error' ? (
          <FailureAlert
            error={current.error}
            onRetry={() => setReload((value) => value + 1)}
          />
        ) : (
          <>
            <UserList
              users={ready?.users ?? []}
              currentUserId={currentUserId}
              pendingId={pendingId}
              isLoading={current === null}
              empty={
                hasFilters ? (
                  <EmptyState
                    title="No users match"
                    action={
                      <Button
                        variant="secondary"
                        size="sm"
                        onClick={clearFilters}
                      >
                        Clear search and filters
                      </Button>
                    }
                  >
                    Try a different name or email, or remove a filter.
                  </EmptyState>
                ) : (
                  <EmptyState title="No users yet">
                    Accounts appear here as people register.
                  </EmptyState>
                )
              }
              onActivate={(user) => changeActive(user, true)}
              onDeactivate={setConfirming}
            />
            {lastPagination && (
              <TablePagination
                page={page}
                totalPages={lastPagination.totalPages}
                totalItems={lastPagination.totalItems}
                onPageChange={goToPage}
              />
            )}
          </>
        )}
      </Section>

      <Modal
        isOpen={confirming !== null}
        onClose={() => setConfirming(null)}
        title={confirming ? `Deactivate ${confirming.fullName}?` : ''}
        description="They are signed out of every device and cannot sign in until you activate the account again."
        actions={
          <>
            <Button
              variant="secondary"
              data-autofocus
              onClick={() => setConfirming(null)}
            >
              Keep active
            </Button>
            <Button
              variant="danger"
              onClick={() => {
                const user = confirming;
                setConfirming(null);
                if (user) void changeActive(user, false);
              }}
            >
              Deactivate
            </Button>
          </>
        }
      />
    </>
  );
}
