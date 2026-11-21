import { useId, type ReactNode } from 'react';

type SectionProps = {
  title: string;
  description?: string;
  /** A link or small button at the end of the heading row, such as "See all". */
  actions?: ReactNode;
  children: ReactNode;
};

/** A heading and its content. Sections sit 48px apart (the shell's gap does it). */
export function Section({
  title,
  description,
  actions,
  children,
}: SectionProps) {
  const headingId = useId();
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="flex flex-col gap-1">
          <h2 id={headingId} className="text-h3 font-semibold">
            {title}
          </h2>
          {description && (
            <p className="text-body-sm text-text-muted">{description}</p>
          )}
        </div>
        {actions}
      </div>
      {children}
    </section>
  );
}
