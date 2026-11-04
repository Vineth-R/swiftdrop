import type { ComponentProps } from 'react';

import { cn } from '../../lib/utils';

function TextareaControl({ className, ...props }: ComponentProps<'textarea'>) {
  return (
    <textarea
      data-slot="textarea"
      className={cn(
        'flex field-sizing-content min-h-16 w-full rounded-control border border-line-control bg-surface px-3 py-2 text-body text-text transition-colors duration-fast ease-out-sd outline-none placeholder:text-text-disabled focus-visible:border-interactive focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-interactive disabled:cursor-not-allowed disabled:border-line disabled:bg-surface-2 disabled:text-text-disabled aria-invalid:border-danger read-only:border-line read-only:bg-surface-2',
        className,
      )}
      {...props}
    />
  );
}

export { TextareaControl };
