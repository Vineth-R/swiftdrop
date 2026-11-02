'use client';

import { SquareX } from 'lucide-react';
import { useEffect, useRef } from 'react';

import { cn } from '../../lib/cn';

export type ErrorSummaryIssue = {
  /** Id of the field, so the item can link to it. */
  fieldId: string;
  /** What the person calls the field, such as "Recipient phone". */
  label: string;
  /** What to do, such as "enter a full number, for example +94 71 234 5678". */
  message: string;
};

type ErrorSummaryProps = {
  issues: ErrorSummaryIssue[];
  /** Shown as "Reference" for 5xx responses only. Never pass one for a 4xx. */
  requestId?: string;
  /** Move focus to the summary when it appears, so the failure is announced. */
  focusOnMount?: boolean;
  className?: string;
};

/**
 * An index of the fields that need attention, shown above a form after a
 * failed submit. It does not replace the messages beside each field.
 */
export function ErrorSummary({
  issues,
  requestId,
  focusOnMount = true,
  className,
}: ErrorSummaryProps) {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (focusOnMount) ref.current?.focus();
  }, [focusOnMount]);

  if (issues.length === 0) return null;
  const count = issues.length;

  return (
    <div
      ref={ref}
      role="alert"
      tabIndex={-1}
      className={cn(
        'flex flex-col gap-3 rounded-card border border-danger-border bg-danger-tint px-5 py-4',
        className,
      )}
    >
      <div className="flex items-center gap-3">
        <SquareX
          size={20}
          strokeWidth={1.5}
          aria-hidden="true"
          className="shrink-0 text-danger"
        />
        <span className="text-body font-semibold text-text">
          {count === 1
            ? 'One field needs attention before you can continue'
            : `${count} fields need attention before you can continue`}
        </span>
      </div>
      <ol className="list-decimal pl-5 text-body-sm text-text">
        {issues.map((issue) => (
          <li key={issue.fieldId} className="py-0.5">
            <a
              href={`#${issue.fieldId}`}
              className="text-interactive underline underline-offset-4 hover:text-interactive-hover"
            >
              {issue.label}
            </a>{' '}
            — {issue.message}
          </li>
        ))}
      </ol>
      {requestId && (
        <span className="select-all font-mono text-overline text-text-faint">
          Reference {requestId}
        </span>
      )}
    </div>
  );
}
