'use client';

import { Lock, RotateCcw } from 'lucide-react';
import { useEffect, useState, type ReactNode } from 'react';

import {
  getAdminUser,
  requestUserPasswordReset,
  setUserActive,
  type AdminUserDetail,
} from '../../lib/admin-users-api';
import { ApiError } from '../../lib/api';
import { formatDateTime } from '../../lib/format';
import { getZones, type Zone } from '../../lib/pricing-api';
import { Alert } from '../ui/alert';
import { Button } from '../ui/button';
import { ConfirmationDialog } from '../ui/confirmation-dialog';
import { KeyValueList } from '../ui/key-value-list';
import { Modal } from '../ui/modal';
import { Skeleton, SkeletonGroup } from '../ui/skeleton';
import { StatusChip } from '../ui/status-chip';
import { EditUserForm } from './edit-user-form';

type State =
  | { status: 'loading' }
  | { status: 'ready'; user: AdminUserDetail }
  | { status: 'error'; error: ApiError };

type Notice = { tone: 'success' | 'warning'; text: string } | null;

function asApiError(error: unknown): ApiError {
  return error instanceof ApiError
    ? error
    : new ApiError({ code: 'INTERNAL', status: 0 });
}

function Group({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-3 border-t border-line pt-5 first:border-t-0 first:pt-0">
      <h3 className="text-h4">{title}</h3>
      {children}
    </section>
  );
}

function providerLabel(provider?: string) {
  if (!provider) return 'Not recorded';
  if (provider === 'LOCAL') return 'Email and password';
  if (provider === 'GOOGLE') return 'Google';
  return provider;
}

function actionMessage(error: ApiError): string {
  if (error.status === 503) {
    return 'Password reset email is not configured right now.';
  }
  switch (error.code) {
    case 'CANNOT_DEACTIVATE_SELF':
      return 'You cannot deactivate your own account. Ask another administrator.';
    case 'ACCOUNT_DEACTIVATED':
      return 'This account is deactivated. Activate it before sending a reset link.';
    case 'PASSWORD_RESET_NOT_SUPPORTED':
      return 'This account signs in with Google, so it has no password to reset.';
    case 'NOT_FOUND':
      return 'This account no longer exists. Close the dialog and reload the list.';
    case 'RATE_LIMITED':
      return error.retryAfter
        ? `Too many requests. Wait ${error.retryAfter} seconds and try again.`
        : 'Too many requests. Wait a moment and try again.';
    case 'NETWORK':
      return 'We could not reach SwiftDrop. Check your connection and try again.';
    default:
      return 'The action could not be completed. Try again in a moment.';
  }
}

type UserDetailsModalProps = {
  userId: string | null;
  currentUserId: string;
  onClose: () => void;
  /** Keeps the list in step after an edit or a status change. */
  onUserChanged: (user: AdminUserDetail) => void;
};

/**
 * The administrative view of one account. It loads the full detail (including
 * the courier profile and authentication provider), and offers edit, status
 * and password-recovery actions. The email and password are never shown.
 */
export function UserDetailsModal({
  userId,
  currentUserId,
  onClose,
  onUserChanged,
}: UserDetailsModalProps) {
  const [state, setState] = useState<State>({ status: 'loading' });
  const [reload, setReload] = useState(0);
  const [view, setView] = useState<'details' | 'edit'>('details');
  const [zones, setZones] = useState<Zone[]>([]);
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<ApiError | null>(null);
  const [notice, setNotice] = useState<Notice>(null);
  const [confirming, setConfirming] = useState<
    'activate' | 'deactivate' | null
  >(null);

  const isOpen = userId !== null;

  useEffect(() => {
    if (!userId) return;
    setView('details');
    setActionError(null);
    setNotice(null);
    setConfirming(null);
  }, [userId]);

  useEffect(() => {
    if (!userId) return;
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

  const user = state.status === 'ready' ? state.user : null;

  useEffect(() => {
    if (!user || user.role !== 'COURIER') {
      setZones([]);
      return;
    }
    const controller = new AbortController();
    getZones(controller.signal)
      .then((list) => {
        if (!controller.signal.aborted) setZones(list);
      })
      .catch(() => {
        if (!controller.signal.aborted) setZones([]);
      });
    return () => controller.abort();
  }, [user?.id, user?.role]);
  const isSelf = user?.id === currentUserId;
  const zoneName = (id?: string | null) =>
    (id && zones.find((zone) => zone.id === id)?.name) || id || null;

  function close() {
    if (busy) return;
    setView('details');
    setConfirming(null);
    onClose();
  }

  function applyUser(updated: AdminUserDetail) {
    setState({ status: 'ready', user: updated });
    onUserChanged(updated);
  }

  async function changeActive(active: boolean) {
    if (!user) return;
    setBusy(true);
    setActionError(null);
    try {
      await setUserActive(user.id, active);
      const updated = { ...user, isActive: active };
      applyUser(updated);
      setNotice({
        tone: 'success',
        text: active
          ? `${user.fullName} can sign in again.`
          : `${user.fullName} is deactivated and signed out of every device.`,
      });
      setConfirming(null);
      setReload((value) => value + 1);
    } catch (error) {
      setActionError(asApiError(error));
    } finally {
      setBusy(false);
    }
  }

  async function resetPassword() {
    if (!user) return;
    setBusy(true);
    setActionError(null);
    setNotice(null);
    try {
      await requestUserPasswordReset(user.id);
      setNotice({
        tone: 'success',
        text: 'Password reset requested. The account will receive a link at its registered email address if it is eligible.',
      });
    } catch (error) {
      setActionError(asApiError(error));
    } finally {
      setBusy(false);
    }
  }

  const title =
    view === 'edit' && user
      ? `Edit ${user.fullName}`
      : user
        ? user.fullName
        : 'User details';
  const description =
    view === 'edit'
      ? 'Change a name, phone number, role or courier details. The email and password are not changed here.'
      : 'Account information and administrative actions.';

  return (
    <>
      <Modal
        isOpen={isOpen}
        onClose={close}
        title={title}
        description={description}
        size="xl"
        isDismissible={!busy}
        actions={
          view === 'edit' ? undefined : (
            <>
              <Button variant="secondary" onClick={close} isDisabled={busy}>
                Close
              </Button>
              {user ? (
                <>
                  <Button
                    variant="secondary"
                    isDisabled={
                      busy ||
                      !user.isActive ||
                      (user.authProvider ?? 'LOCAL') !== 'LOCAL'
                    }
                    isLoading={busy}
                    onClick={() => void resetPassword()}
                  >
                    Reset password
                  </Button>
                  <Button
                    variant="secondary"
                    isDisabled={busy}
                    onClick={() => {
                      setActionError(null);
                      setNotice(null);
                      setView('edit');
                    }}
                  >
                    Edit user
                  </Button>
                  {user.isActive ? (
                    <Button
                      variant="danger"
                      isDisabled={busy || isSelf}
                      onClick={() => {
                        setActionError(null);
                        setConfirming('deactivate');
                      }}
                    >
                      Deactivate
                    </Button>
                  ) : (
                    <Button
                      isDisabled={busy}
                      onClick={() => {
                        setActionError(null);
                        setConfirming('activate');
                      }}
                    >
                      Activate
                    </Button>
                  )}
                </>
              ) : null}
            </>
          )
        }
      >
        {state.status === 'loading' ? (
          <SkeletonGroup label="Loading user details">
            <div className="flex flex-col gap-4">
              <Skeleton className="h-6 w-1/3" />
              <Skeleton className="h-4 w-full" />
              <Skeleton className="h-4 w-5/6" />
              <Skeleton className="h-4 w-2/3" />
            </div>
          </SkeletonGroup>
        ) : state.status === 'error' ? (
          <Alert
            tone={state.error.code === 'FORBIDDEN' ? 'warning' : 'danger'}
            icon={state.error.code === 'FORBIDDEN' ? Lock : undefined}
            title={
              state.error.code === 'FORBIDDEN'
                ? 'You do not have access to this'
                : state.error.code === 'NOT_FOUND'
                  ? 'This account no longer exists'
                  : 'Could not load this account'
            }
            actions={
              <Button
                variant="secondary"
                size="sm"
                onClick={() => setReload((value) => value + 1)}
              >
                Try again
              </Button>
            }
          >
            {actionMessage(state.error)}
          </Alert>
        ) : view === 'edit' && user ? (
          <EditUserForm
            user={user}
            onCancel={() => setView('details')}
            onSaved={(updated) => {
              applyUser(updated);
              setView('details');
              setReload((value) => value + 1);
            }}
          />
        ) : user ? (
          <div className="flex flex-col gap-5">
            {notice ? <Alert tone={notice.tone} title={notice.text} /> : null}
            {actionError ? (
              <Alert tone="danger" title="Could not complete the action">
                {actionMessage(actionError)}
              </Alert>
            ) : null}
            {isSelf ? (
              <Alert tone="info" title="This is your account">
                You cannot deactivate your own account or change your own role.
              </Alert>
            ) : null}
            {user.isActive && (user.authProvider ?? 'LOCAL') !== 'LOCAL' ? (
              <Alert tone="info" title="This account does not use a password">
                It signs in with {providerLabel(user.authProvider)}. There is no
                password to reset.
              </Alert>
            ) : null}
            {!user.isActive ? (
              <Alert tone="warning" title="This account is deactivated">
                Activate it before sending a password reset link.
              </Alert>
            ) : null}

            <Group title="Account information">
              <KeyValueList
                layout="inline"
                items={[
                  { label: 'Full name', value: user.fullName },
                  { label: 'Email address', value: user.email, isMono: true },
                  {
                    label: 'Phone number',
                    value: user.phone,
                    isMono: true,
                  },
                  { label: 'User ID', value: user.id, isMono: true },
                  { label: 'Role', value: user.role },
                  {
                    label: 'Account status',
                    value: (
                      <StatusChip
                        status={user.isActive ? 'ACTIVE' : 'DEACTIVATED'}
                      />
                    ),
                  },
                  {
                    label: 'Created',
                    value: formatDateTime(user.createdAt),
                    isMono: true,
                  },
                  {
                    label: 'Last updated',
                    value: formatDateTime(user.updatedAt),
                    isMono: true,
                  },
                  {
                    label: 'Last login',
                    value: user.lastLoginAt
                      ? formatDateTime(user.lastLoginAt)
                      : 'Never',
                    isMono: Boolean(user.lastLoginAt),
                  },
                  {
                    label: 'Authentication provider',
                    value: providerLabel(user.authProvider),
                  },
                ]}
              />
            </Group>

            {user.role === 'COURIER' ? (
              <Group title="Courier details">
                <KeyValueList
                  layout="inline"
                  items={[
                    {
                      label: 'Vehicle type',
                      value: user.courierProfile?.vehicleType ?? null,
                    },
                    {
                      label: 'Vehicle number',
                      value: user.courierProfile?.vehicleNumber ?? null,
                      isMono: true,
                    },
                    {
                      label: 'Home zone',
                      value: zoneName(user.courierProfile?.homeZoneId),
                    },
                    {
                      label: 'Availability',
                      value: user.courierProfile?.isAvailable
                        ? 'Available for assignments'
                        : 'Not available',
                    },
                  ]}
                />
              </Group>
            ) : null}

            <Group title="Administrative actions">
              <p className="text-body-sm text-text-muted">
                A password reset sends a single-use link to the account's
                registered email. Existing sessions end only after the person
                chooses a new password.
              </p>
              <div className="flex flex-wrap gap-2">
                <Button
                  variant="secondary"
                  size="sm"
                  isDisabled={
                    busy ||
                    !user.isActive ||
                    (user.authProvider ?? 'LOCAL') !== 'LOCAL'
                  }
                  isLoading={busy}
                  onClick={() => void resetPassword()}
                >
                  <RotateCcw size={16} strokeWidth={1.5} aria-hidden="true" />
                  Initiate password reset
                </Button>
              </div>
            </Group>
          </div>
        ) : null}
      </Modal>

      <ConfirmationDialog
        isOpen={confirming === 'deactivate'}
        onClose={() => setConfirming(null)}
        title={user ? `Deactivate ${user.fullName}?` : 'Deactivate account?'}
        description="They are signed out of every device and cannot sign in until you activate the account again."
        confirmLabel="Deactivate"
        cancelLabel="Keep active"
        tone="danger"
        isLoading={busy}
        onConfirm={() => void changeActive(false)}
        error={actionError ? actionMessage(actionError) : undefined}
      />
      <ConfirmationDialog
        isOpen={confirming === 'activate'}
        onClose={() => setConfirming(null)}
        title={user ? `Activate ${user.fullName}?` : 'Activate account?'}
        description="The account can sign in again after activation."
        confirmLabel="Activate"
        tone="primary"
        isLoading={busy}
        onConfirm={() => void changeActive(true)}
        error={actionError ? actionMessage(actionError) : undefined}
      />
    </>
  );
}
