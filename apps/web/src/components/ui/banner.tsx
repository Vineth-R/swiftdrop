import { CircleAlert, Clock, Info, X, type LucideIcon } from 'lucide-react';
import type { ReactNode } from 'react';

import { cn } from '../../lib/cn';

export type BannerTone = 'info' | 'warning' | 'danger';

const tones: Record<
  BannerTone,
  { box: string; icon: string; Icon: LucideIcon }
> = {
  info: {
    box: 'border-info-border bg-info-tint',
    icon: 'text-info',
    Icon: Info,
  },
  warning: {
    box: 'border-warning-border bg-warning-tint',
    icon: 'text-warning',
    Icon: Clock,
  },
  danger: {
    box: 'border-danger-border bg-danger-tint',
    icon: 'text-danger',
    Icon: CircleAlert,
  },
};

type BannerProps = {
  tone: BannerTone;
  /** The bold first sentence. */
  title: string;
  /** The rest, in the muted colour. */
  children?: ReactNode;
  /** A link or button at the end, such as "Check status". */
  action?: ReactNode;
  /** Shows a close button. Only info banners should be dismissible. */
  onDismiss?: () => void;
  className?: string;
};

/**
 * A page-level notice that sits between the top bar and the page header. It has
 * a bottom border instead of a full outline, so it reads as part of the page
 * chrome. Show one at a time; a more urgent one (danger, then warning, then
 * info) replaces a calmer one.
 */
export function Banner({
  tone,
  title,
  children,
  action,
  onDismiss,
  className,
}: BannerProps) {
  const { box, icon, Icon } = tones[tone];
  return (
    <div
      role={tone === 'danger' ? 'alert' : 'status'}
      className={cn(
        'flex items-center gap-3 border-b px-4 py-3 md:px-8',
        box,
        className,
      )}
    >
      <Icon
        size={18}
        strokeWidth={1.5}
        aria-hidden="true"
        className={cn('shrink-0', icon)}
      />
      <p className="flex-1 text-body-sm">
        <span className="font-semibold text-text">{title}</span>
        {children && <span className="text-text-muted"> {children}</span>}
      </p>
      {action}
      {onDismiss && (
        <button
          type="button"
          aria-label="Dismiss banner"
          onClick={onDismiss}
          className="inline-flex size-7 shrink-0 items-center justify-center rounded-chip text-text-muted hover:bg-surface-2"
        >
          <X size={16} strokeWidth={1.5} aria-hidden="true" />
        </button>
      )}
    </div>
  );
}
