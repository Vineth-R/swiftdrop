import {
  Ban,
  Circle,
  CircleCheck,
  Clock,
  MapPin,
  Package,
  SquareX,
  Truck,
  type LucideIcon,
} from 'lucide-react';

import { cn } from '../../lib/cn';
import type { ParcelStatus } from '../../lib/tokens';

/** The merchant approval states (API 8.4 to 8.8). */
export type MerchantStatus = 'PENDING' | 'APPROVED' | 'REJECTED' | 'SUSPENDED';

/** Whether an account can sign in (API 2.10 to 2.14, `isActive`). */
export type AccountStatus = 'ACTIVE' | 'DEACTIVATED';

export type ChipStatus = ParcelStatus | MerchantStatus | AccountStatus;

type Look = { icon: LucideIcon; box: string };

const neutral =
  'border-status-created-bd bg-status-created-bg text-status-created-fg';

// Every look is a full class string so Tailwind can find it. The icon differs
// per status, so a status never relies on colour alone (rule 04).
const looks: Record<ChipStatus, Look> = {
  CREATED: { icon: Circle, box: neutral },
  PICKED_UP: {
    icon: Package,
    box: 'border-status-picked-up-bd bg-status-picked-up-bg text-status-picked-up-fg',
  },
  IN_TRANSIT: {
    icon: Truck,
    box: 'border-status-in-transit-bd bg-status-in-transit-bg text-status-in-transit-fg',
  },
  OUT_FOR_DELIVERY: {
    icon: MapPin,
    box: 'border-status-out-bd bg-status-out-bg text-status-out-fg',
  },
  DELIVERED: {
    icon: CircleCheck,
    box: 'border-status-delivered-bd bg-status-delivered-bg text-status-delivered-fg',
  },
  FAILED: {
    icon: SquareX,
    box: 'border-status-failed-bd bg-status-failed-bg text-status-failed-fg',
  },
  PENDING: {
    icon: Clock,
    box: 'border-warning-border bg-warning-tint text-warning',
  },
  APPROVED: {
    icon: CircleCheck,
    box: 'border-success-border bg-success-tint text-success',
  },
  REJECTED: {
    icon: SquareX,
    box: 'border-danger-border bg-danger-tint text-danger',
  },
  SUSPENDED: { icon: Ban, box: neutral },
  ACTIVE: {
    icon: CircleCheck,
    box: 'border-success-border bg-success-tint text-success',
  },
  DEACTIVATED: { icon: Ban, box: neutral },
};

const sizes = {
  sm: 'h-5 gap-1 px-1.5',
  md: 'h-6 gap-1.5 px-2',
} as const;

type StatusChipProps = {
  status: ChipStatus;
  size?: keyof typeof sizes;
  /**
   * Only the shape, for a very tight cell. It keeps the status name for screen
   * readers and as a tooltip, so it is never a bare coloured dot.
   */
  isCompact?: boolean;
  className?: string;
};

/** "OUT_FOR_DELIVERY" as the chip writes it: "OUT FOR DELIVERY". */
export function statusLabel(status: ChipStatus): string {
  return status.replaceAll('_', ' ');
}

/**
 * A status as a chip: a shape and its name in capitals. The six parcel
 * statuses are the only parcel statuses there are; the merchant and account
 * states share the chip so every list reads the same way.
 */
export function StatusChip({
  status,
  size = 'md',
  isCompact,
  className,
}: StatusChipProps) {
  const { icon: Icon, box } = looks[status];
  const label = statusLabel(status);

  if (isCompact) {
    return (
      <span
        title={label}
        className={cn(
          'inline-flex size-6 items-center justify-center rounded-chip border',
          box,
          className,
        )}
      >
        <Icon size={12} strokeWidth={1.5} aria-hidden="true" />
        <span className="sr-only">{label}</span>
      </span>
    );
  }

  return (
    <span
      className={cn(
        'inline-flex w-fit items-center whitespace-nowrap rounded-chip border text-overline font-semibold tracking-[0.04em]',
        sizes[size],
        box,
        className,
      )}
    >
      <Icon size={12} strokeWidth={1.5} aria-hidden="true" />
      {label}
    </span>
  );
}
