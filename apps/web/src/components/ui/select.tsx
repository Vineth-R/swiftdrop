import { ChevronDown } from 'lucide-react';
import type { ComponentProps, ReactNode } from 'react';

import { cn } from '../../lib/utils';
import { Field, controlClasses, describedBy, type ControlSize } from './field';

export type SelectOption = {
  value: string;
  label: string;
  isDisabled?: boolean;
};

type SelectProps = Omit<
  ComponentProps<'select'>,
  'id' | 'size' | 'children'
> & {
  id: string;
  label: ReactNode;
  options: SelectOption[];
  /**
   * The first, unchosen entry, such as "Choose a zone". It cannot be picked
   * again once something else is chosen. Without it the first option is chosen.
   */
  placeholder?: string;
  helper?: ReactNode;
  error?: ReactNode;
  success?: ReactNode;
  isRequired?: boolean;
  labelAction?: ReactNode;
  controlSize?: ControlSize;
};

/**
 * One choice from a list. It is the browser's own select, so the keyboard,
 * the screen reader and the phone's picker all work as people expect; only the
 * closed control is styled.
 */
export function Select({
  id,
  label,
  options,
  placeholder,
  helper,
  error,
  success,
  isRequired,
  labelAction,
  controlSize = 'md',
  disabled,
  className,
  ...select
}: SelectProps) {
  const isUnchosen = placeholder !== undefined && select.value === '';

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
      <div className="relative">
        <select
          id={id}
          disabled={disabled}
          aria-invalid={error ? true : undefined}
          aria-required={isRequired || undefined}
          aria-describedby={describedBy(id, { helper, error, success })}
          {...select}
          className={cn(
            controlClasses({
              size: controlSize,
              hasError: Boolean(error),
              hasSuccess: Boolean(success),
              isSelect: true,
            }),
            'appearance-none pr-10',
            isUnchosen && 'text-text-faint',
          )}
        >
          {placeholder !== undefined && (
            <option value="" disabled>
              {placeholder}
            </option>
          )}
          {options.map((option) => (
            <option
              key={option.value}
              value={option.value}
              disabled={option.isDisabled}
            >
              {option.label}
            </option>
          ))}
        </select>
        <ChevronDown
          size={16}
          strokeWidth={1.5}
          aria-hidden="true"
          className={cn(
            'pointer-events-none absolute right-3 top-1/2 -translate-y-1/2',
            disabled ? 'text-text-disabled' : 'text-text-muted',
          )}
        />
      </div>
    </Field>
  );
}
