import {
  CircleCheck,
  Info,
  SquareX,
  TriangleAlert,
  type LucideIcon,
} from 'lucide-react';
import type { ReactNode, Ref } from 'react';

import { cn } from '../../lib/cn';

export type AlertTone = 'info' | 'success' | 'warning' | 'danger';

/** The icon of each tone. Exported so a Toast shows the same shapes as an Alert. */
export const alertIcons: Record<AlertTone, LucideIcon> = {
  info: Info,
  success: CircleCheck,
  warning: TriangleAlert,
  // A square, not a circle: tone survives greyscale because the shape differs too.
  danger: SquareX,
};

/** The border, tint and icon colour of each tone. */
export const alertToneClasses: Record<
  AlertTone,
  { box: string; icon: string }
> = {
  info: { box: 'border-info-border bg-info-tint', icon: 'text-info' },
  success: {
    box: 'border-success-border bg-success-tint',
    icon: 'text-success',
  },
  warning: {
    box: 'border-warning-border bg-warning-tint',
    icon: 'text-warning',
  },
  danger: { box: 'border-danger-border bg-danger-tint', icon: 'text-danger' },
};

type AlertProps = {
  tone: AlertTone;
  title: ReactNode;
  children?: ReactNode;
  /** Buttons or links under the message. */
  actions?: ReactNode;
  /** Replaces the tone's icon, such as a lock for a locked account. */
  icon?: LucideIcon;
  className?: string;
  /** Lets a form move focus here when a failed submit shows the alert. */
  ref?: Ref<HTMLDivElement>;
};

/**
 * Says what happened and what to do next. Danger interrupts screen readers
 * (role="alert"); the other tones wait their turn (role="status").
 */
export function Alert({
  tone,
  title,
  children,
  actions,
  icon,
  className,
  ref,
}: AlertProps) {
  const Icon = icon ?? alertIcons[tone];
  return (
    <div
      ref={ref}
      tabIndex={ref ? -1 : undefined}
      role={tone === 'danger' ? 'alert' : 'status'}
      className={cn(
        'flex gap-3 rounded-card border px-4 py-3',
        alertToneClasses[tone].box,
        className,
      )}
    >
      <Icon
        size={20}
        strokeWidth={1.5}
        aria-hidden="true"
        className={cn('mt-0.5 shrink-0', alertToneClasses[tone].icon)}
      />
      <div className="flex min-w-0 flex-col gap-1">
        <div className="text-body-sm font-semibold text-text">{title}</div>
        {children && (
          <div className="text-body-sm text-text-muted">{children}</div>
        )}
        {actions && <div className="mt-1 flex flex-wrap gap-3">{actions}</div>}
      </div>
    </div>
  );
}
