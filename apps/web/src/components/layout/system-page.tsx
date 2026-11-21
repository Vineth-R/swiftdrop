import type { ReactNode } from 'react';

import { RouteLine } from '../ui/route-line';

type SystemPageProps = {
  /** The status in small caps, such as "404 · Not found". */
  label: string;
  /** One sentence that says what happened. */
  title: string;
  /** One or two sentences that say what to do. */
  children: ReactNode;
  /** The one way out, as buttons. */
  actions?: ReactNode;
  /** The API's request ID. Pass it for a fault on our side only. */
  reference?: string;
  /**
   * 1 for a real page, which is the page's one heading. A style-guide demo sits
   * under the guide's own h1, so it passes 2.
   */
  headingLevel?: 1 | 2;
};

/**
 * 404, 403, 500 and the offline page. Each names what happened in one sentence,
 * offers the one route out, and shows a Reference line only when there is a
 * request ID worth quoting. No giant numeral, no mascot, no apology.
 */
export function SystemPage({
  label,
  title,
  children,
  actions,
  reference,
  headingLevel = 1,
}: SystemPageProps) {
  const Heading = headingLevel === 2 ? 'h2' : 'h1';
  return (
    <div className="mx-auto flex w-full max-w-md flex-1 flex-col justify-center gap-6 py-10">
      <RouteLine state="idle" />
      <div className="flex flex-col gap-2">
        <span className="text-overline uppercase text-text-faint">{label}</span>
        <Heading className="font-display text-h2 md:text-h1">{title}</Heading>
        <p className="text-body text-text-muted">{children}</p>
      </div>
      {actions && (
        <div className="flex flex-col gap-3 sm:flex-row">{actions}</div>
      )}
      {reference && (
        <p className="select-all break-all font-mono text-overline text-text-faint">
          Reference {reference}
        </p>
      )}
    </div>
  );
}
