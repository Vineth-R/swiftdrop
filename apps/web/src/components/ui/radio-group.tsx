'use client';

import { CircleIcon } from 'lucide-react';
import type { ComponentProps, ReactNode } from 'react';
import { RadioGroup as RadioGroupPrimitive } from 'radix-ui';

import { cn } from '../../lib/utils';
import { ChoiceGroup, ChoiceRow, type ChoiceOption } from './choice';

/**
 * The bare shadcn/Radix radio group for custom compositions. The labelled
 * SwiftDrop `RadioGroup` below is the form control; this is the primitive.
 */
function RadioGroupControl({
  className,
  ...props
}: ComponentProps<typeof RadioGroupPrimitive.Root>) {
  return (
    <RadioGroupPrimitive.Root
      data-slot="radio-group"
      className={cn('grid gap-3', className)}
      {...props}
    />
  );
}

function RadioGroupItem({
  className,
  ...props
}: ComponentProps<typeof RadioGroupPrimitive.Item>) {
  return (
    <RadioGroupPrimitive.Item
      data-slot="radio-group-item"
      className={cn(
        'aspect-square size-4 shrink-0 rounded-full border border-line-control bg-surface text-primary shadow-xs transition-[color,box-shadow] outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20',
        className,
      )}
      {...props}
    >
      <RadioGroupPrimitive.Indicator
        data-slot="radio-group-indicator"
        className="relative flex items-center justify-center"
      >
        <CircleIcon className="absolute top-1/2 left-1/2 size-2 -translate-x-1/2 -translate-y-1/2 fill-primary" />
      </RadioGroupPrimitive.Indicator>
    </RadioGroupPrimitive.Item>
  );
}

type RadioGroupProps = {
  /** Shared by every radio, and the prefix of the option ids. */
  name: string;
  legend: ReactNode;
  options: ChoiceOption[];
  /** The chosen value, or '' when nothing is chosen yet. */
  value: string;
  onChange: (value: string) => void;
  helper?: ReactNode;
  error?: ReactNode;
  isRequired?: boolean;
  isDisabled?: boolean;
  /** Horizontal fits two or three short options from tablet width up. */
  orientation?: 'vertical' | 'horizontal';
  className?: string;
};

/**
 * Exactly one choice from a short list, all visible at once, such as the
 * parcel size. For a long list use Select. The arrow keys move between the
 * options, as the browser does for radios.
 */
export function RadioGroup({
  name,
  legend,
  options,
  value,
  onChange,
  helper,
  error,
  isRequired,
  isDisabled,
  orientation,
  className,
}: RadioGroupProps) {
  return (
    <ChoiceGroup
      id={name}
      legend={legend}
      helper={helper}
      error={error}
      isRequired={isRequired}
      isDisabled={isDisabled}
      orientation={orientation}
      className={className}
    >
      {options.map((option) => (
        <ChoiceRow
          key={option.value}
          id={`${name}-${option.value}`}
          type="radio"
          name={name}
          value={option.value}
          label={option.label}
          description={option.description}
          disabled={option.isDisabled}
          hasError={Boolean(error)}
          checked={value === option.value}
          onChange={() => onChange(option.value)}
        />
      ))}
    </ChoiceGroup>
  );
}

export { RadioGroupControl, RadioGroupItem };
