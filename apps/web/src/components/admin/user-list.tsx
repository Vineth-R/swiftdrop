'use client';

import { Ban, CircleCheck } from 'lucide-react';
import { useState } from 'react';

import type { AdminUser } from '../../lib/admin-users-api';
import { Alert } from '../ui/alert';
import { Button } from '../ui/button';
import { RoleBadge } from '../ui/role-badge';

const joinedFormat = new Intl.DateTimeFormat('en-GB', {
  dateStyle: 'medium',
  timeZone: 'UTC',
});

function joined(createdAt: string): string {
  const date = new Date(createdAt);
  return Number.isNaN(date.getTime()) ? '' : joinedFormat.format(date);
}

/** Active or deactivated as an icon and a word, never colour alone. */
function ActiveStatus({ isActive }: { isActive: boolean }) {
  const Icon = isActive ? CircleCheck : Ban;
  return (
    <span
      className={
        isActive
          ? 'inline-flex items-center gap-1.5 text-body-sm text-success'
          : 'inline-flex items-center gap-1.5 text-body-sm text-text-muted'
      }
    >
      <Icon size={16} strokeWidth={1.5} aria-hidden="true" />
      {isActive ? 'Active' : 'Deactivated'}
    </span>
  );
}

type UserRowProps = {
  user: AdminUser;
  isSelf: boolean;
  isPending: boolean;
  onSetActive: (user: AdminUser, isActive: boolean) => void;
};

function UserRow({ user, isSelf, isPending, onSetActive }: UserRowProps) {
  const [isConfirming, setIsConfirming] = useState(false);
  const since = joined(user.createdAt);

  return (
    <li className="flex flex-col gap-3 border-t px-4 py-3 first:border-t-0">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 flex-col gap-1">
          <span className="text-body font-semibold text-text">
            {user.fullName}
            {isSelf && (
              <span className="ml-2 text-caption font-normal text-text-muted">
                (you)
              </span>
            )}
          </span>
          <span className="break-all font-mono text-caption text-text-muted">
            {user.email}
          </span>
          {since && (
            <span className="text-caption text-text-faint">Joined {since}</span>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <RoleBadge role={user.role} />
          <ActiveStatus isActive={user.isActive} />
          {user.isActive ? (
            <Button
              variant="secondary"
              size="sm"
              isLoading={isPending}
              isDisabled={isSelf}
              onClick={() => setIsConfirming(true)}
              aria-label={`Deactivate ${user.fullName}`}
            >
              Deactivate
            </Button>
          ) : (
            <Button
              variant="secondary"
              size="sm"
              isLoading={isPending}
              onClick={() => onSetActive(user, true)}
              aria-label={`Activate ${user.fullName}`}
            >
              Activate
            </Button>
          )}
        </div>
      </div>
      {isSelf && user.isActive && (
        <p className="text-caption text-text-muted">
          You cannot deactivate your own account.
        </p>
      )}
      {isConfirming && user.isActive && (
        <Alert
          tone="warning"
          title={`Deactivate ${user.fullName}?`}
          actions={
            <>
              <Button
                variant="danger"
                size="sm"
                onClick={() => {
                  setIsConfirming(false);
                  onSetActive(user, false);
                }}
              >
                Deactivate
              </Button>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setIsConfirming(false)}
              >
                Keep active
              </Button>
            </>
          }
        >
          They are signed out of every device and cannot sign in until you
          activate the account again.
        </Alert>
      )}
    </li>
  );
}

type UserListProps = {
  users: AdminUser[];
  currentUserId: string;
  pendingId: string | null;
  onSetActive: (user: AdminUser, isActive: boolean) => void;
};

/**
 * TODO(design-foundation): a plain list until the kit has a DataTable,
 * StatusChip and Modal. Not exported for reuse: swap it for those, do not
 * build on it.
 */
export function UserList({
  users,
  currentUserId,
  pendingId,
  onSetActive,
}: UserListProps) {
  return (
    <ul className="rounded-card border bg-surface" aria-label="Users">
      {users.map((user) => (
        <UserRow
          key={user.id}
          user={user}
          isSelf={user.id === currentUserId}
          isPending={pendingId === user.id}
          onSetActive={onSetActive}
        />
      ))}
    </ul>
  );
}
