import { Check, CircleAlert } from 'lucide-react';
import type { ComponentProps, ReactNode } from 'react';

import { cn } from '@/lib/utils';
import { Input } from './input';
import { Label } from './label';

export type ControlSize = 'md' | 'lg';

type FieldMessages = {
  helper?: ReactNode;
  error?: ReactNode;
  success?: ReactNode;
};

/** Ids of the message elements, so a control can point at them. */
export function messageId(id: string, kind: 'helper' | 'error' | 'success') {
  return `${id}-${kind}`;
}

/** The aria-describedby value for a control: the message that is showing. */
export function describedBy(
  id: string,
  { helper, error, success }: FieldMessages,
  extra?: string,
): string | undefined {
  const message = error
    ? messageId(id, 'error')
    : success
      ? messageId(id, 'success')
      : helper
        ? messageId(id, 'helper')
        : undefined;
  const joined = [message, extra].filter(Boolean).join(' ');
  return joined || undefined;
}

/**
 * The line under a control: the error if there is one, else the success, else
 * the helper. An error replaces the helper instead of stacking under it, so the
 * field never changes height twice. Its id is what `describedBy` points at.
 */
export function FieldMessage({
  id,
  helper,
  error,
  success,
}: FieldMessages & { id: string }) {
  if (error) {
    return (
      <span
        id={messageId(id, 'error')}
        className="flex items-center gap-1.5 text-caption text-danger"
      >
        <CircleAlert size={14} strokeWidth={1.5} aria-hidden="true" />
        {error}
      </span>
    );
  }
  if (success) {
    return (
      <span
        id={messageId(id, 'success')}
        className="flex items-center gap-1.5 text-caption text-success"
      >
        <Check size={14} strokeWidth={1.5} aria-hidden="true" />
        {success}
      </span>
    );
  }
  if (helper) {
    return (
      <span
        id={messageId(id, 'helper')}
        className="text-caption text-text-muted"
      >
        {helper}
      </span>
    );
  }
  return null;
}

type FieldProps = FieldMessages & {
  /** Id of the control this label belongs to. Required, so the label is always real. */
  id: string;
  label: ReactNode;
  isRequired?: boolean;
  isDisabled?: boolean;
  /** Right of the label, such as "Forgot password?". */
  labelAction?: ReactNode;
  className?: string;
  children: ReactNode;
};

/**
 * Label, control and message. An error replaces the helper text rather than
 * stacking under it, so the field never changes height twice.
 */
export function Field({
  id,
  label,
  helper,
  error,
  success,
  isRequired,
  isDisabled,
  labelAction,
  className,
  children,
}: FieldProps) {
  return (
    <div className={cn('flex flex-col gap-2', className)}>
      <div className="flex items-baseline justify-between gap-3">
        <Label
          htmlFor={id}
          className={cn(
            'text-label',
            isDisabled ? 'text-text-disabled' : 'text-text',
          )}
        >
          {label}
          {isRequired && (
            <>
              <span aria-hidden="true" className="text-danger">
                {' '}
                *
              </span>
              <span className="sr-only"> (required)</span>
            </>
          )}
        </Label>
        {labelAction}
      </div>
      {children}
      <FieldMessage id={id} helper={helper} error={error} success={success} />
    </div>
  );
}

const heights: Record<ControlSize, string> = {
  md: 'h-10 pointer-coarse:h-11',
  // 48px on phones, 44px from tablet up; text is 16px on phones so iOS does not zoom.
  lg: 'h-12 md:h-11',
};

const text: Record<ControlSize, string> = {
  md: 'text-body',
  lg: 'text-body-lg md:text-body',
};

/** Looks of a lone input, and of the bordered wrapper around a prefixed one. */
export function controlClasses({
  size = 'md',
  hasError = false,
  hasSuccess = false,
  isGroup = false,
  isMultiline = false,
  isSelect = false,
}: {
  size?: ControlSize;
  hasError?: boolean;
  hasSuccess?: boolean;
  isGroup?: boolean;
  /** A textarea: no fixed height, a taller minimum and vertical padding. */
  isMultiline?: boolean;
  /** A select. Browsers treat it as read-only, so it must not take the read-only look. */
  isSelect?: boolean;
}): string {
  const border = hasError
    ? 'border-danger'
    : hasSuccess
      ? 'border-success'
      : 'border-line-control hover:border-text-faint';
  return cn(
    'w-full rounded-control border bg-surface text-text',
    isMultiline ? 'min-h-24 resize-y py-2' : heights[size],
    border,
    isGroup
      ? 'flex items-stretch overflow-hidden focus-within:outline-2 focus-within:outline-offset-2 focus-within:outline-interactive'
      : cn(
          'px-3 transition-colors duration-fast ease-out-sd focus-visible:border-interactive',
          text[size],
          'disabled:border-line disabled:bg-surface-2 disabled:text-text-disabled',
          !isSelect && 'read-only:border-line read-only:bg-surface-2',
        ),
  );
}

/** Looks of the bare input inside a group, which borrows the wrapper's border and ring. */
export function groupInputClasses(size: ControlSize = 'md'): string {
  return cn(
    'h-full min-w-0 flex-1 bg-transparent px-3 text-text focus-visible:outline-none',
    text[size],
  );
}

export type InputProps = Omit<ComponentProps<'input'>, 'size' | 'id'>;

type TextFieldProps = InputProps &
  FieldMessages & {
    id: string;
    label: ReactNode;
    isRequired?: boolean;
    labelAction?: ReactNode;
    /** md is 40px. lg is the auth and touch size. */
    controlSize?: ControlSize;
    /** Use for identifiers and amounts: tracking numbers, keys, money. */
    isMono?: boolean;
    /** Fixed text before the input, such as a currency or country code. */
    prefix?: ReactNode;
    suffix?: ReactNode;
  };

export function TextField({
  id,
  label,
  helper,
  error,
  success,
  isRequired,
  labelAction,
  controlSize = 'md',
  isMono,
  prefix,
  suffix,
  className,
  disabled,
  ...input
}: TextFieldProps) {
  const isGroup = Boolean(prefix || suffix);
  const common = {
    id,
    disabled,
    'aria-invalid': error ? true : undefined,
    'aria-required': isRequired || undefined,
    'aria-describedby': describedBy(id, { helper, error, success }),
    ...input,
  };
  const mono = isMono && 'font-mono tabular text-button';

  return (
    <Field
      id={id}
      label={label}
      helper={helper}
      error={error}
      success={success}
      isRequired={isRequired}
      isDisabled={disabled}
      labelAction={labelAction}
      className={className}
    >
      {isGroup ? (
        <div
          className={controlClasses({
            size: controlSize,
            hasError: Boolean(error),
            hasSuccess: Boolean(success),
            isGroup: true,
          })}
        >
          {prefix && (
            <span className="inline-flex items-center border-r bg-surface-2 px-3 font-mono text-label text-text-muted">
              {prefix}
            </span>
          )}
          <Input
            {...common}
            className={cn(
              groupInputClasses(controlSize),
              'h-full rounded-none border-0 bg-transparent px-3 py-0 shadow-none focus-visible:outline-none',
              mono,
            )}
          />
          {suffix && (
            <span className="inline-flex items-center border-l bg-surface-2 px-3 text-caption text-text-muted">
              {suffix}
            </span>
          )}
        </div>
      ) : (
        <Input
          {...common}
          className={cn(
            controlClasses({
              size: controlSize,
              hasError: Boolean(error),
              hasSuccess: Boolean(success),
            }),
            mono,
          )}
        />
      )}
    </Field>
  );
}
