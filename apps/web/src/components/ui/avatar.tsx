'use client';

import { Avatar as AvatarPrimitive } from 'radix-ui';

import { initialsOf } from '../../lib/initials';
import { cn } from '../../lib/utils';

/**
 * Kept as a re-export for existing client-side imports. Server components can
 * import the helper directly from lib/initials without crossing the client
 * boundary created by Radix Avatar.
 */
export { initialsOf };

const sizes = {
  sm: 'size-7 text-overline',
  md: 'size-8 text-caption',
} as const;

type AvatarProps = {
  name: string;
  size?: keyof typeof sizes;
  className?: string;
};

/** Initials on an Ink circle. It is decorative: the name is always written next to it. */
export function Avatar({ name, size = 'md', className }: AvatarProps) {
  return (
    <AvatarPrimitive.Root
      aria-hidden="true"
      data-slot="avatar"
      className={cn(
        'inline-flex shrink-0 items-center justify-center overflow-hidden rounded-full',
        sizes[size],
        className,
      )}
    >
      <AvatarPrimitive.Fallback className="flex size-full items-center justify-center rounded-full bg-primary font-semibold text-on-primary">
        {initialsOf(name)}
      </AvatarPrimitive.Fallback>
    </AvatarPrimitive.Root>
  );
}
