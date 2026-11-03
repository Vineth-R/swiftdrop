import { cn } from '../../lib/cn';
import type { Role } from '../../lib/tokens';

const looks: Record<Role, { box: string; initial: string }> = {
  CUSTOMER: {
    box: 'border-status-created-bd bg-status-created-bg text-status-created-fg',
    initial: 'bg-status-created-fg text-status-created-bg',
  },
  MERCHANT: {
    box: 'border-info-border bg-info-tint text-info',
    initial: 'bg-info text-info-tint',
  },
  COURIER: {
    box: 'border-interactive-border bg-interactive-tint text-interactive',
    initial: 'bg-interactive text-interactive-tint',
  },
  DISPATCHER: {
    box: 'border-warning-border bg-warning-tint text-warning',
    initial: 'bg-warning text-warning-tint',
  },
  // The one badge with a strong fill, so an admin is never mistaken for staff.
  ADMIN: {
    box: 'border-primary bg-primary text-on-primary',
    initial: 'bg-accent text-on-accent',
  },
};

const sizes = {
  sm: {
    box: 'h-5 gap-1.5 pl-1 pr-2 text-overline',
    initial: 'size-3.5 text-[9px]',
  },
  md: {
    box: 'h-6 gap-1.5 pl-1 pr-2 text-overline',
    initial: 'size-4 text-[10px]',
  },
} as const;

type RoleBadgeProps = {
  role: Role;
  size?: keyof typeof sizes;
  className?: string;
};

/** The role as a small label: a lettered square and the name, never colour alone. */
export function RoleBadge({ role, size = 'md', className }: RoleBadgeProps) {
  const look = looks[role];
  return (
    <span
      className={cn(
        'inline-flex w-fit items-center rounded-chip border font-semibold tracking-[0.04em]',
        sizes[size].box,
        look.box,
        className,
      )}
    >
      <span
        aria-hidden="true"
        className={cn(
          'inline-flex items-center justify-center rounded-[4px] font-mono',
          sizes[size].initial,
          look.initial,
        )}
      >
        {role[0]}
      </span>
      {role}
    </span>
  );
}
