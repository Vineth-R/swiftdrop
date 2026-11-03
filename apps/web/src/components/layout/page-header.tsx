import type { ReactNode } from 'react';

type PageHeaderProps = {
  title: string;
  /** One sentence about what this page is for. */
  description?: string;
  /** At most two buttons, and only one of them primary. */
  actions?: ReactNode;
};

/** Every page starts with this: the title, a line about it, and its actions. */
export function PageHeader({ title, description, actions }: PageHeaderProps) {
  return (
    <header className="flex flex-wrap items-end justify-between gap-x-6 gap-y-4">
      <div className="flex min-w-0 flex-col gap-2">
        <h1 className="font-display text-h2 md:text-h1">{title}</h1>
        {description && (
          <p className="max-w-prose text-body text-text-muted">{description}</p>
        )}
      </div>
      {actions && <div className="flex flex-wrap gap-3">{actions}</div>}
    </header>
  );
}
