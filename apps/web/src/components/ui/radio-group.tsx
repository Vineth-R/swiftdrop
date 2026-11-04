'use client';

import type { ReactNode } from 'react';

import { ChoiceGroup, ChoiceRow, type ChoiceOption } from './choice';

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
