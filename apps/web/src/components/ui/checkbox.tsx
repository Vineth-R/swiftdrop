'use client';

import { CheckIcon } from 'lucide-react';
import type { ComponentProps, ReactNode } from 'react';
import { Checkbox as CheckboxPrimitive } from 'radix-ui';

import { cn } from '../../lib/utils';
import { ChoiceGroup, ChoiceRow, type ChoiceOption } from './choice';
import { FieldMessage, describedBy } from './field';

/**
 * The bare shadcn/Radix checkbox for custom compositions. The labelled
 * SwiftDrop `Checkbox` below is the form control; this is the primitive.
 */
function CheckboxControl({
  className,
  ...props
}: ComponentProps<typeof CheckboxPrimitive.Root>) {
  return (
    <CheckboxPrimitive.Root
      data-slot="checkbox"
      className={cn(
        'peer size-4 shrink-0 rounded-chip border border-line-control bg-surface shadow-xs transition-shadow outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 data-[state=checked]:border-primary data-[state=checked]:bg-primary data-[state=checked]:text-primary-foreground',
        className,
      )}
      {...props}
    >
      <CheckboxPrimitive.Indicator
        data-slot="checkbox-indicator"
        className="grid place-content-center text-current transition-none"
      >
        <CheckIcon className="size-3" />
      </CheckboxPrimitive.Indicator>
    </CheckboxPrimitive.Root>
  );
}

type CheckboxProps = Omit<
  ComponentProps<'input'>,
  'type' | 'id' | 'size' | 'children'
> & {
  id: string;
  label: ReactNode;
  description?: ReactNode;
  helper?: ReactNode;
  error?: ReactNode;
  isRequired?: boolean;
  /** "Some, not all": for a select-all box above a list. It is a look, not a value. */
  isIndeterminate?: boolean;
};

/**
 * One yes or no, such as "I accept the terms". For several related choices use
 * CheckboxGroup, so they share a legend. A checkbox that saves instantly belongs
 * in a settings row, not a form.
 */
export function Checkbox({
  id,
  label,
  description,
  helper,
  error,
  isRequired,
  isIndeterminate,
  className,
  ...input
}: CheckboxProps) {
  return (
    <div className={cn('flex flex-col gap-1', className)}>
      <ChoiceRow
        {...input}
        id={id}
        type="checkbox"
        label={label}
        description={description}
        isRequired={isRequired}
        isIndeterminate={isIndeterminate}
        hasError={Boolean(error)}
        aria-describedby={describedBy(id, { helper, error })}
      />
      <FieldMessage id={id} helper={helper} error={error} />
    </div>
  );
}

type CheckboxGroupProps = {
  /** Shared by every box, and the prefix of the option ids. */
  name: string;
  legend: ReactNode;
  options: ChoiceOption[];
  /** The values that are ticked. */
  values: string[];
  onChange: (values: string[]) => void;
  helper?: ReactNode;
  error?: ReactNode;
  isRequired?: boolean;
  isDisabled?: boolean;
  className?: string;
};

/** Several related choices under one legend, such as the events a webhook sends. */
export function CheckboxGroup({
  name,
  legend,
  options,
  values,
  onChange,
  helper,
  error,
  isRequired,
  isDisabled,
  className,
}: CheckboxGroupProps) {
  return (
    <ChoiceGroup
      id={name}
      legend={legend}
      helper={helper}
      error={error}
      isRequired={isRequired}
      isDisabled={isDisabled}
      className={className}
    >
      {options.map((option) => (
        <ChoiceRow
          key={option.value}
          id={`${name}-${option.value}`}
          type="checkbox"
          name={name}
          value={option.value}
          label={option.label}
          description={option.description}
          disabled={option.isDisabled}
          hasError={Boolean(error)}
          checked={values.includes(option.value)}
          onChange={(event) =>
            onChange(
              event.target.checked
                ? [...values, option.value]
                : values.filter((value) => value !== option.value),
            )
          }
        />
      ))}
    </ChoiceGroup>
  );
}

export { CheckboxControl };
