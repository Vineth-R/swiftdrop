import type { ComponentProps, ReactNode } from 'react';

import { Field, controlClasses, describedBy, type ControlSize } from './field';
import { TextareaControl } from './textarea-control';

type TextareaProps = Omit<ComponentProps<'textarea'>, 'id'> & {
  id: string;
  label: ReactNode;
  helper?: ReactNode;
  error?: ReactNode;
  success?: ReactNode;
  isRequired?: boolean;
  labelAction?: ReactNode;
  controlSize?: ControlSize;
  /**
   * Shows "12/200" beside the label. It needs `maxLength` and a controlled
   * `value`, because it counts what the person has typed.
   */
  showCount?: boolean;
};

/**
 * Several lines of text, such as the reason for rejecting a merchant. It takes
 * the same label, helper, error and success as TextField, so a form mixes the
 * two without a visible seam.
 */
export function Textarea({
  id,
  label,
  helper,
  error,
  success,
  isRequired,
  labelAction,
  controlSize = 'md',
  showCount,
  rows = 4,
  disabled,
  className,
  ...textarea
}: TextareaProps) {
  const count =
    showCount && textarea.maxLength ? (
      <span className="font-mono text-caption tabular text-text-muted">
        {String(textarea.value ?? '').length}/{textarea.maxLength}
      </span>
    ) : (
      labelAction
    );

  return (
    <Field
      id={id}
      label={label}
      helper={helper}
      error={error}
      success={success}
      isRequired={isRequired}
      isDisabled={disabled}
      labelAction={count}
      className={className}
    >
      <TextareaControl
        id={id}
        rows={rows}
        disabled={disabled}
        aria-invalid={error ? true : undefined}
        aria-required={isRequired || undefined}
        aria-describedby={describedBy(id, { helper, error, success })}
        {...textarea}
        className={controlClasses({
          size: controlSize,
          hasError: Boolean(error),
          hasSuccess: Boolean(success),
          isMultiline: true,
        })}
      />
    </Field>
  );
}
