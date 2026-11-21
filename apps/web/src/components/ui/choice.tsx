'use client';

import { useEffect, useRef, type ComponentProps, type ReactNode } from 'react';

import { cn } from '../../lib/cn';
import { FieldMessage, describedBy } from './field';

export type ChoiceOption = {
  value: string;
  label: ReactNode;
  /** One short line under the label, such as what a delivery speed costs. */
  description?: ReactNode;
  isDisabled?: boolean;
};

type ChoiceRowProps = Omit<
  ComponentProps<'input'>,
  'type' | 'id' | 'size' | 'children'
> & {
  id: string;
  type: 'checkbox' | 'radio';
  label: ReactNode;
  description?: ReactNode;
  isRequired?: boolean;
  isIndeterminate?: boolean;
  hasError?: boolean;
};

/**
 * One checkbox or radio with its label. The whole row is the target and is at
 * least 44px high, so it works with a thumb. It is the browser's own input,
 * coloured by the primary token, so keyboard and screen readers behave as usual.
 */
export function ChoiceRow({
  id,
  type,
  label,
  description,
  isRequired,
  isIndeterminate,
  hasError,
  disabled,
  className,
  ...input
}: ChoiceRowProps) {
  const ref = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (ref.current) ref.current.indeterminate = Boolean(isIndeterminate);
  }, [isIndeterminate]);

  return (
    <label
      htmlFor={id}
      className={cn(
        'flex min-h-11 items-start gap-3 py-2',
        disabled ? 'cursor-not-allowed text-text-disabled' : 'cursor-pointer',
        className,
      )}
    >
      <input
        {...input}
        ref={ref}
        id={id}
        type={type}
        disabled={disabled}
        aria-invalid={hasError ? true : undefined}
        aria-required={isRequired || undefined}
        className={cn(
          'mt-0.5 size-5 shrink-0 accent-primary',
          disabled ? 'cursor-not-allowed' : 'cursor-pointer',
        )}
      />
      <span className="flex min-w-0 flex-col gap-0.5">
        <span className="text-body">
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
        </span>
        {description && (
          <span
            className={cn(
              'text-body-sm',
              disabled ? 'text-text-disabled' : 'text-text-muted',
            )}
          >
            {description}
          </span>
        )}
      </span>
    </label>
  );
}

type ChoiceGroupProps = {
  /** Prefix of the message ids. Use the group's `name`. */
  id: string;
  legend: ReactNode;
  helper?: ReactNode;
  error?: ReactNode;
  isRequired?: boolean;
  isDisabled?: boolean;
  orientation?: 'vertical' | 'horizontal';
  className?: string;
  children: ReactNode;
};

/** The fieldset and legend a set of checkboxes or radios belongs in. */
export function ChoiceGroup({
  id,
  legend,
  helper,
  error,
  isRequired,
  isDisabled,
  orientation = 'vertical',
  className,
  children,
}: ChoiceGroupProps) {
  return (
    <fieldset
      disabled={isDisabled}
      aria-describedby={describedBy(id, { helper, error })}
      className={cn('m-0 flex min-w-0 flex-col gap-1 border-0 p-0', className)}
    >
      <legend
        className={cn(
          'mb-1 p-0 text-label',
          isDisabled ? 'text-text-disabled' : 'text-text',
        )}
      >
        {legend}
        {isRequired && (
          <>
            <span aria-hidden="true" className="text-danger">
              {' '}
              *
            </span>
            <span className="sr-only"> (required)</span>
          </>
        )}
      </legend>
      <div
        className={cn(
          'flex',
          orientation === 'horizontal'
            ? 'flex-col md:flex-row md:flex-wrap md:gap-x-6'
            : 'flex-col',
        )}
      >
        {children}
      </div>
      <FieldMessage id={id} helper={helper} error={error} />
    </fieldset>
  );
}
