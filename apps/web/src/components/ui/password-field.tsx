'use client';

import { Check } from 'lucide-react';
import { useEffect, useId, useMemo, useState, type ChangeEvent } from 'react';

import { cn } from '@/lib/utils';
import {
  checkPassword,
  passwordByteLength,
  passwordCharacterLength,
  PASSWORD_MAX_BYTES,
  strengthLabels,
} from '@/lib/password';
import { Button } from './button';
import { Input } from './input';
import {
  controlClasses,
  describedBy,
  Field,
  groupInputClasses,
  type ControlSize,
  type InputProps,
} from './field';

type PasswordChecklistProps = {
  value: string;
  /** Id for the list, so an input can name it in aria-describedby. */
  id: string;
  className?: string;
};

/**
 * Four bars, one line of text and the four rules. The rules are guidance, not
 * errors, so they update as the person types and are announced politely.
 */
export function PasswordChecklist({
  value,
  id,
  className,
}: PasswordChecklistProps) {
  const { rules, score } = useMemo(() => checkPassword(value), [value]);
  const characters = useMemo(() => passwordCharacterLength(value), [value]);
  const bytes = useMemo(() => passwordByteLength(value), [value]);
  const fill =
    score === 4 ? 'bg-success' : score >= 2 ? 'bg-warning' : 'bg-danger';

  return (
    <div className={cn('flex flex-col gap-2', className)}>
      <div className="flex gap-1" aria-hidden="true">
        {[0, 1, 2, 3].map((segment) => (
          <span
            key={segment}
            className={cn(
              'h-1.5 flex-1 rounded-chip',
              segment < score ? fill : 'bg-line',
            )}
          />
        ))}
      </div>
      <div className="flex items-baseline justify-between gap-3">
        <span className="text-caption text-text-muted">
          {strengthLabels[score]}
        </span>
        <span className="tabular font-mono text-overline text-text-faint">
          {characters} characters · {bytes} / {PASSWORD_MAX_BYTES} bytes
        </span>
      </div>
      <ul
        id={id}
        aria-live="polite"
        className="flex flex-col gap-2 rounded-control bg-surface-2 px-4 py-3"
      >
        {rules.map((rule) => (
          <li key={rule.id} className="flex items-center gap-2">
            {rule.isMet ? (
              <span
                aria-hidden="true"
                className="inline-flex size-4 items-center justify-center rounded-full bg-success text-success-tint"
              >
                <Check size={11} strokeWidth={2.5} />
              </span>
            ) : (
              <span
                aria-hidden="true"
                className="size-4 rounded-full border-[1.5px] border-dashed border-line-control"
              />
            )}
            <span
              className={cn(
                'text-body-sm',
                rule.isMet ? 'text-text' : 'text-text-muted',
              )}
            >
              {rule.label}
            </span>
            <span className="sr-only">
              {rule.isMet ? 'met' : 'not met yet'}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}

type PasswordFieldProps = Omit<InputProps, 'type' | 'autoComplete'> & {
  id: string;
  label: string;
  /** Tells password managers what to do: sign in, or choose a new password. */
  autoComplete: 'current-password' | 'new-password';
  helper?: string;
  error?: string;
  isRequired?: boolean;
  labelAction?: React.ReactNode;
  controlSize?: ControlSize;
  /** Show the bars and the four rules. Use on new-password fields only. */
  showChecklist?: boolean;
  /** Called with the number of rules met, 0 to 4, whenever it changes. */
  onStrengthChange?: (score: number) => void;
};

export function PasswordField({
  id,
  label,
  autoComplete,
  helper,
  error,
  isRequired,
  labelAction,
  controlSize = 'md',
  showChecklist = false,
  onStrengthChange,
  value: controlledValue,
  defaultValue,
  onChange,
  disabled,
  ...input
}: PasswordFieldProps) {
  const rulesId = `${useId()}-rules`;
  const [isRevealed, setIsRevealed] = useState(false);
  const [ownValue, setOwnValue] = useState(String(defaultValue ?? ''));
  const value =
    controlledValue === undefined ? ownValue : String(controlledValue);
  const { score } = useMemo(() => checkPassword(value), [value]);

  useEffect(() => {
    onStrengthChange?.(score);
  }, [score, onStrengthChange]);

  function handleChange(event: ChangeEvent<HTMLInputElement>) {
    setOwnValue(event.target.value);
    onChange?.(event);
  }

  return (
    <Field
      id={id}
      label={label}
      helper={helper}
      error={error}
      isRequired={isRequired}
      isDisabled={disabled}
      labelAction={labelAction}
    >
      <div
        className={controlClasses({
          size: controlSize,
          hasError: Boolean(error),
          isGroup: true,
        })}
      >
        <Input
          {...input}
          id={id}
          type={isRevealed ? 'text' : 'password'}
          autoComplete={autoComplete}
          value={value}
          onChange={handleChange}
          disabled={disabled}
          aria-invalid={error ? true : undefined}
          aria-required={isRequired || undefined}
          aria-describedby={describedBy(
            id,
            { helper, error },
            showChecklist ? rulesId : undefined,
          )}
          className={cn(
            groupInputClasses(controlSize),
            'h-full rounded-none border-0 bg-transparent px-3 py-0 shadow-none focus-visible:outline-none',
          )}
        />
        <Button
          variant="link"
          size="sm"
          isDisabled={disabled}
          aria-label={isRevealed ? 'Hide password' : 'Show password'}
          aria-pressed={isRevealed}
          onClick={() => setIsRevealed((current) => !current)}
          className="h-full rounded-none border-l px-3 text-caption hover:decoration-1"
        >
          {isRevealed ? 'Hide' : 'Show'}
        </Button>
      </div>
      {showChecklist && <PasswordChecklist id={rulesId} value={value} />}
    </Field>
  );
}
