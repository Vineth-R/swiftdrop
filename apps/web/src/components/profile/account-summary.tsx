import type { User } from '../../lib/auth-api';
import { Avatar } from '../ui/avatar';
import { RoleBadge } from '../ui/role-badge';

/** The API stores times in UTC; show the date in Sri Lanka, whatever the browser's zone. */
const dateFormat = new Intl.DateTimeFormat('en-GB', {
  dateStyle: 'long',
  timeZone: 'Asia/Colombo',
});

function memberSince(createdAt?: string): string | undefined {
  if (!createdAt) return undefined;
  const date = new Date(createdAt);
  return Number.isNaN(date.getTime()) ? undefined : dateFormat.format(date);
}

/** Who this account is, read-only. The form below it changes what can change. */
export function AccountSummary({ user }: { user: User }) {
  const since = memberSince(user.createdAt);
  return (
    <div className="flex max-w-xl flex-col gap-5 rounded-card border border-line bg-surface p-5">
      <div className="flex items-center gap-4">
        <Avatar name={user.fullName} className="size-14 text-body-lg" />
        <div className="flex min-w-0 flex-col gap-1">
          <span className="truncate text-h3 font-semibold">
            {user.fullName}
          </span>
          <span className="truncate font-mono text-body-sm text-text-muted">
            {user.email}
          </span>
        </div>
      </div>
      <dl className="grid grid-cols-1 gap-4 border-t border-line pt-4 sm:grid-cols-3">
        <div className="flex flex-col gap-1">
          <dt className="text-overline uppercase text-text-faint">Role</dt>
          <dd>
            <RoleBadge role={user.role} />
          </dd>
        </div>
        <div className="flex flex-col gap-1">
          <dt className="text-overline uppercase text-text-faint">
            Signs in with
          </dt>
          <dd className="text-body-sm">
            {user.authProvider === 'GOOGLE' ? 'Google' : 'Email and password'}
          </dd>
        </div>
        {since && (
          <div className="flex flex-col gap-1">
            <dt className="text-overline uppercase text-text-faint">
              Member since
            </dt>
            <dd className="text-body-sm">{since}</dd>
          </div>
        )}
      </dl>
    </div>
  );
}
