'use client';

import { useState, type ComponentProps, type ReactNode } from 'react';

import { cn } from '../../lib/cn';
import { buttonClasses } from './button';
import { Field, describedBy, type ControlSize } from './field';

type FileFieldProps = Omit<
  ComponentProps<'input'>,
  'type' | 'id' | 'size' | 'value' | 'onChange'
> & {
  id: string;
  label: ReactNode;
  helper?: ReactNode;
  error?: ReactNode;
  isRequired?: boolean;
  /** The text of the button. Default "Choose a file". */
  buttonLabel?: string;
  /** What shows beside the button until a file is chosen. Default "No file chosen". */
  emptyLabel?: string;
  /** lg is the 48px courier size. */
  controlSize?: ControlSize;
  /**
   * Called with the chosen file, or null when it is cleared. Check the type and
   * the size yourself and pass your own sentence as `error`: the browser's
   * `accept` is only a hint.
   */
  onFileChange: (file: File | null) => void;
};

/**
 * Picks one file, such as a delivery proof photo. The real file input sits
 * invisibly over the button, so a click, the keyboard and a phone's camera or
 * gallery all work, and the focus ring shows on the button. On a phone,
 * `accept="image/*"` with `capture="environment"` opens the camera.
 */
export function FileField({
  id,
  label,
  helper,
  error,
  isRequired,
  buttonLabel = 'Choose a file',
  emptyLabel = 'No file chosen',
  controlSize = 'md',
  onFileChange,
  disabled,
  className,
  ...input
}: FileFieldProps) {
  const [name, setName] = useState<string | null>(null);

  return (
    <Field
      id={id}
      label={label}
      helper={helper}
      error={error}
      isRequired={isRequired}
      isDisabled={disabled}
      className={className}
    >
      <div className="flex items-center gap-3">
        <span className="relative inline-flex shrink-0">
          <input
            {...input}
            id={id}
            type="file"
            disabled={disabled}
            aria-invalid={error ? true : undefined}
            aria-required={isRequired || undefined}
            aria-describedby={describedBy(id, { helper, error })}
            onChange={(event) => {
              const file = event.target.files?.[0] ?? null;
              setName(file?.name ?? null);
              onFileChange(file);
            }}
            className="peer absolute inset-0 size-full cursor-pointer opacity-0 disabled:cursor-not-allowed"
          />
          <span
            aria-hidden="true"
            className={cn(
              buttonClasses({
                variant: 'secondary',
                size: controlSize === 'lg' ? 'lg' : 'md',
                isUnavailable: disabled,
              }),
              'peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-interactive',
              Boolean(error) && 'border-danger',
            )}
          >
            {buttonLabel}
          </span>
        </span>
        <span
          className={cn(
            'min-w-0 truncate text-body-sm',
            name ? 'text-text' : 'text-text-muted',
          )}
        >
          {name ?? emptyLabel}
        </span>
      </div>
    </Field>
  );
}
