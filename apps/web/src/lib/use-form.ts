'use client';

import { useCallback, useMemo, useState } from 'react';

type Values = Record<string, string>;
export type FormRules<V extends Values> = {
  [K in keyof V]: (values: V) => string | undefined;
};
type Errors<V extends Values> = Partial<Record<keyof V, string>>;

/**
 * Field state with the validation timing of the design system:
 * - nothing is validated before a field has been left once;
 * - after a failed submit, every field validates on each change, so an error
 *   clears the moment it is fixed;
 * - a message from the server sits in the same slot as a client message and
 *   disappears as soon as that field is edited.
 *
 * Define `rules` once outside the component so it stays the same object.
 */
export function useForm<V extends Values>(initial: V, rules: FormRules<V>) {
  const [values, setValues] = useState<V>(initial);
  const [touched, setTouched] = useState<Partial<Record<keyof V, boolean>>>({});
  const [submitted, setSubmitted] = useState(false);
  const [serverErrors, setServerErrors] = useState<Errors<V>>({});

  const errors = useMemo(() => {
    const result: Errors<V> = {};
    for (const key of Object.keys(rules) as (keyof V)[]) {
      const message =
        serverErrors[key] ??
        (touched[key] || submitted ? rules[key](values) : undefined);
      if (message) result[key] = message;
    }
    return result;
  }, [rules, serverErrors, touched, submitted, values]);

  const setValue = useCallback((key: keyof V, value: string) => {
    setValues((current) => ({ ...current, [key]: value }));
    setServerErrors((current) => {
      if (!(key in current)) return current;
      const next = { ...current };
      delete next[key];
      return next;
    });
  }, []);

  const blur = useCallback(
    (key: keyof V) => setTouched((current) => ({ ...current, [key]: true })),
    [],
  );

  /** Marks the form as submitted and reports the problems found right now. */
  const validate = useCallback((): Errors<V> => {
    setSubmitted(true);
    setServerErrors({});
    const found: Errors<V> = {};
    for (const key of Object.keys(rules) as (keyof V)[]) {
      const message = rules[key](values);
      if (message) found[key] = message;
    }
    return found;
  }, [rules, values]);

  /**
   * Forget which fields were left and that a submit failed, so cleared fields
   * stay quiet until the person touches them again.
   */
  const resetValidation = useCallback(() => {
    setTouched({});
    setSubmitted(false);
  }, []);

  return {
    values,
    errors,
    setValue,
    blur,
    validate,
    setServerErrors,
    resetValidation,
  };
}
