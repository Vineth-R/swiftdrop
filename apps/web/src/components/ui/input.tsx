import type { ComponentProps } from 'react';

import { cn } from '../../lib/utils';

function Input({ className, type, ...props }: ComponentProps<'input'>) {
  return (
    <input
      type={type}
      data-slot="input"
      className={cn(
        'h-10 w-full min-w-0 rounded-control border border-line-control bg-surface px-3 py-1 text-body text-text transition-colors duration-fast ease-out-sd outline-none file:inline-flex file:h-7 file:border-0 file:bg-transparent file:text-body-sm file:font-medium file:text-text placeholder:text-text-disabled selection:bg-primary selection:text-on-primary disabled:border-line disabled:bg-surface-2 disabled:text-text-disabled read-only:border-line read-only:bg-surface-2',
        'focus-visible:border-interactive focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-interactive',
        'aria-invalid:border-danger',
        className,
      )}
      {...props}
    />
  );
}

export { Input };
