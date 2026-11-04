import { cva } from 'class-variance-authority';
import Link from 'next/link';
import type { ComponentProps, ReactNode } from 'react';

import { cn } from '../../lib/utils';
import { Spinner } from './spinner';

export type ButtonVariant =
  'primary' | 'secondary' | 'accent' | 'ghost' | 'danger' | 'link';
export type ButtonSize = 'sm' | 'md' | 'lg';

const base =
  'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-control border font-semibold transition-colors duration-fast ease-out-sd';

const variants: Record<ButtonVariant, string> = {
  primary:
    'border-primary bg-primary text-on-primary hover:border-primary-hover hover:bg-primary-hover active:border-primary-active active:bg-primary-active',
  secondary:
    'border-line-control bg-surface text-text hover:border-text-faint hover:bg-surface-2 active:bg-surface-3',
  accent:
    'border-accent bg-accent text-on-accent hover:border-accent-hover hover:bg-accent-hover active:bg-accent-active',
  ghost:
    'border-transparent bg-transparent text-text hover:bg-surface-2 active:bg-surface-3',
  danger:
    'border-danger bg-danger text-on-danger hover:border-danger-hover hover:bg-danger-hover active:border-danger-active active:bg-danger-active',
  link: '',
};

/** Disabled looks. A loading button keeps its normal fill and only gains the spinner. */
const disabledLook: Record<ButtonVariant, string> = {
  primary: 'border-line bg-line text-text-disabled',
  secondary: 'border-line bg-transparent text-text-disabled',
  accent: 'border-line bg-accent/30 text-text-disabled',
  ghost: 'border-transparent bg-transparent text-text-disabled',
  danger: 'border-line bg-line text-text-disabled',
  link: 'text-text-disabled',
};

const sizes: Record<ButtonSize, string> = {
  sm: 'h-8 px-3 text-body-sm',
  md: 'h-10 px-4 text-button',
  lg: 'h-12 px-5 text-body',
};

const linkLook =
  'inline-flex items-center gap-1.5 text-button font-medium text-interactive underline underline-offset-4 hover:text-interactive-hover hover:decoration-2';

const buttonVariants = cva(base, {
  variants: {
    variant: {
      primary: variants.primary,
      secondary: variants.secondary,
      accent: variants.accent,
      ghost: variants.ghost,
      danger: variants.danger,
    },
    size: {
      sm: sizes.sm,
      md: sizes.md,
      lg: sizes.lg,
    },
    fullWidth: {
      true: 'w-full',
    },
    unavailable: {
      true: 'cursor-not-allowed',
    },
  },
  compoundVariants: [
    { variant: 'primary', unavailable: true, class: disabledLook.primary },
    { variant: 'secondary', unavailable: true, class: disabledLook.secondary },
    { variant: 'accent', unavailable: true, class: disabledLook.accent },
    { variant: 'ghost', unavailable: true, class: disabledLook.ghost },
    { variant: 'danger', unavailable: true, class: disabledLook.danger },
  ],
  defaultVariants: {
    variant: 'primary',
    size: 'md',
  },
});

type StyleOptions = {
  variant?: ButtonVariant;
  size?: ButtonSize;
  isFullWidth?: boolean;
  isUnavailable?: boolean;
};

/** Class names for anything that has to look like a button, such as a link. */
export function buttonClasses({
  variant = 'primary',
  size = 'md',
  isFullWidth = false,
  isUnavailable = false,
}: StyleOptions = {}): string {
  if (variant === 'link') {
    return cn(linkLook, isUnavailable && disabledLook.link);
  }
  return cn(
    buttonVariants({
      variant,
      size,
      fullWidth: isFullWidth,
      unavailable: isUnavailable,
    }),
    // Ghost buttons are tighter on the left and right (Buttons board).
    variant === 'ghost' && size === 'md' && 'px-3',
  );
}

type ButtonProps = Omit<ComponentProps<'button'>, 'disabled'> & {
  variant?: ButtonVariant;
  size?: ButtonSize;
  /** Shows the spinner and ignores clicks. Keep the label meaningful, such as "Saving". */
  isLoading?: boolean;
  isDisabled?: boolean;
  isFullWidth?: boolean;
  iconLeading?: ReactNode;
  iconTrailing?: ReactNode;
};

/**
 * One primary per view. A loading button stays focusable and keeps its place
 * in the layout (aria-disabled, not `disabled`), and it stops being a submit
 * button so a second click cannot post the form twice.
 */
export function Button({
  variant = 'primary',
  size = 'md',
  isLoading = false,
  isDisabled = false,
  isFullWidth = false,
  iconLeading,
  iconTrailing,
  type = 'button',
  className,
  children,
  onClick,
  ...rest
}: ButtonProps) {
  return (
    <button
      {...rest}
      type={isLoading ? 'button' : type}
      disabled={isDisabled}
      aria-busy={isLoading || undefined}
      aria-disabled={isLoading || undefined}
      onClick={isLoading ? undefined : onClick}
      data-slot="button"
      data-variant={variant}
      data-size={size}
      className={cn(
        buttonClasses({
          variant,
          size,
          isFullWidth,
          isUnavailable: isDisabled,
        }),
        isLoading && 'cursor-progress',
        isDisabled && 'cursor-not-allowed',
        className,
      )}
    >
      {isLoading ? (
        <Spinner tone={variant === 'primary' ? 'on-primary' : 'current'} />
      ) : (
        iconLeading
      )}
      {children}
      {!isLoading && iconTrailing}
    </button>
  );
}

type ButtonLinkProps = Omit<ComponentProps<typeof Link>, 'className'> &
  Pick<StyleOptions, 'variant' | 'size' | 'isFullWidth'> & {
    className?: string;
  };

/** A link that looks like a button, for navigation rather than actions. */
export function ButtonLink({
  variant = 'primary',
  size = 'md',
  isFullWidth,
  className,
  ...rest
}: ButtonLinkProps) {
  return (
    <Link
      {...rest}
      data-slot="button-link"
      data-variant={variant}
      data-size={size}
      className={cn(buttonClasses({ variant, size, isFullWidth }), className)}
    />
  );
}
