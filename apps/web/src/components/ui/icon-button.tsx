import type { LucideIcon } from 'lucide-react';
import type { ComponentProps } from 'react';

import { cn } from '../../lib/cn';
import {
  buttonBase,
  buttonDisabledClasses,
  buttonVariantClasses,
  type ButtonVariant,
} from './button';
import { Spinner } from './spinner';

export type IconButtonVariant = Extract<
  ButtonVariant,
  'ghost' | 'secondary' | 'danger'
>;

const sizes = {
  sm: 'size-9',
  // 44px, the smallest comfortable target. Use lg (48px) on courier screens.
  md: 'size-11',
  lg: 'size-12',
} as const;

type IconButtonProps = Omit<
  ComponentProps<'button'>,
  'disabled' | 'children' | 'aria-label'
> & {
  icon: LucideIcon;
  /**
   * What the button does and to what, such as "Actions for SDK7M2Q9XP4". It is
   * the accessible name and the tooltip, so it is required and it never repeats
   * the icon's shape ("Trash").
   */
  label: string;
  variant?: IconButtonVariant;
  size?: keyof typeof sizes;
  /** Shows the spinner and ignores clicks, like Button. */
  isLoading?: boolean;
  isDisabled?: boolean;
};

/**
 * An action with no visible text, such as a row's menu or a close button. Prefer
 * a Button with a word when there is room. Several next to each other need 8px
 * between them (`gap-2`).
 */
export function IconButton({
  icon: Icon,
  label,
  variant = 'ghost',
  size = 'md',
  isLoading = false,
  isDisabled = false,
  type = 'button',
  className,
  onClick,
  ...rest
}: IconButtonProps) {
  return (
    <button
      {...rest}
      type={isLoading ? 'button' : type}
      title={label}
      aria-label={label}
      disabled={isDisabled}
      aria-busy={isLoading || undefined}
      aria-disabled={isLoading || undefined}
      onClick={isLoading ? undefined : onClick}
      className={cn(
        buttonBase,
        'shrink-0 p-0',
        sizes[size],
        isDisabled
          ? buttonDisabledClasses[variant]
          : buttonVariantClasses[variant],
        isLoading && 'cursor-progress',
        isDisabled && 'cursor-not-allowed',
        className,
      )}
    >
      {isLoading ? (
        <Spinner size={20} />
      ) : (
        <Icon size={20} strokeWidth={1.5} aria-hidden="true" />
      )}
    </button>
  );
}
