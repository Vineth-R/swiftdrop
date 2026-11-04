'use client';

import type { ReactNode } from 'react';

import type { AdminUser } from '../../lib/admin-users-api';
import { formatDate } from '../../lib/format';
import { Button } from '../ui/button';
import { DataTable, type DataTableColumn } from '../ui/data-table';
import { RoleBadge } from '../ui/role-badge';
import { StatusChip } from '../ui/status-chip';

type UserListProps = {
  users: AdminUser[];
  currentUserId: string;
  /** The user whose status change is running. */
  pendingId: string | null;
  isLoading: boolean;
  /** An `EmptyState` for when there are no rows. */
  empty: ReactNode;
  onActivate: (user: AdminUser) => void;
  /** Asks first: the screen opens its confirmation dialog. */
  onDeactivate: (user: AdminUser) => void;
};

/** The users as a table, and as cards below 768px. */
export function UserList({
  users,
  currentUserId,
  pendingId,
  isLoading,
  empty,
  onActivate,
  onDeactivate,
}: UserListProps) {
  const columns: DataTableColumn<AdminUser>[] = [
    {
      key: 'fullName',
      header: 'Name',
      cell: (user) => (
        <>
          {user.fullName}
          {user.id === currentUserId && (
            <span className="ml-2 text-caption font-normal text-text-muted">
              (you)
            </span>
          )}
        </>
      ),
    },
    {
      key: 'email',
      header: 'Email',
      cell: (user) => <span className="break-all">{user.email}</span>,
      isMono: true,
    },
    {
      key: 'role',
      header: 'Role',
      cell: (user) => <RoleBadge role={user.role} />,
    },
    {
      key: 'status',
      header: 'Status',
      cell: (user) => (
        <StatusChip status={user.isActive ? 'ACTIVE' : 'DEACTIVATED'} />
      ),
    },
    {
      key: 'createdAt',
      header: 'Joined',
      cell: (user) => formatDate(user.createdAt),
    },
    {
      key: 'actions',
      header: 'Actions',
      isHeaderHidden: true,
      cell: (user) => {
        const isSelf = user.id === currentUserId;
        if (!user.isActive) {
          return (
            <Button
              variant="secondary"
              size="sm"
              isLoading={pendingId === user.id}
              onClick={() => onActivate(user)}
              aria-label={`Activate ${user.fullName}`}
            >
              Activate
            </Button>
          );
        }
        return (
          <div className="flex flex-col items-start gap-1">
            <Button
              variant="secondary"
              size="sm"
              isLoading={pendingId === user.id}
              isDisabled={isSelf}
              onClick={() => onDeactivate(user)}
              aria-label={`Deactivate ${user.fullName}`}
            >
              Deactivate
            </Button>
            {isSelf && (
              <span className="text-caption text-text-muted">
                You cannot deactivate your own account.
              </span>
            )}
          </div>
        );
      },
    },
  ];

  return (
    <DataTable
      caption="Users"
      columns={columns}
      rows={users}
      getRowId={(user) => user.id}
      isLoading={isLoading}
      empty={empty}
    />
  );
}
