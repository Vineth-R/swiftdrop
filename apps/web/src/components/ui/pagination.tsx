import {
  ChevronLeftIcon,
  ChevronRightIcon,
  MoreHorizontalIcon,
} from 'lucide-react';
import type { ComponentProps } from 'react';

import { cn } from '../../lib/utils';
import { buttonClasses, type ButtonSize } from './button';

function Pagination({ className, ...props }: ComponentProps<'nav'>) {
  return (
    <nav
      role="navigation"
      aria-label="Page navigation"
      data-slot="pagination"
      className={cn('mx-auto flex w-full justify-center', className)}
      {...props}
    />
  );
}

function PaginationContent({ className, ...props }: ComponentProps<'ul'>) {
  return (
    <ul
      data-slot="pagination-content"
      className={cn('flex flex-row items-center gap-1', className)}
      {...props}
    />
  );
}

function PaginationItem(props: ComponentProps<'li'>) {
  return <li data-slot="pagination-item" {...props} />;
}

type PaginationLinkProps = ComponentProps<'button'> & {
  isActive?: boolean;
  size?: ButtonSize;
};

function PaginationLink({
  className,
  isActive,
  size = 'sm',
  type = 'button',
  ...props
}: PaginationLinkProps) {
  return (
    <button
      type={type}
      aria-current={isActive ? 'page' : undefined}
      data-slot="pagination-link"
      data-active={isActive}
      className={cn(
        buttonClasses({
          variant: isActive ? 'secondary' : 'ghost',
          size,
        }),
        'w-10 px-0 disabled:pointer-events-none disabled:opacity-50',
        className,
      )}
      {...props}
    />
  );
}

function PaginationPrevious({
  className,
  ...props
}: ComponentProps<typeof PaginationLink>) {
  return (
    <PaginationLink
      aria-label="Go to previous page"
      className={cn('w-auto gap-1 px-3', className)}
      {...props}
    >
      <ChevronLeftIcon size={16} strokeWidth={1.5} aria-hidden="true" />
      <span className="hidden sm:block">Previous</span>
    </PaginationLink>
  );
}

function PaginationNext({
  className,
  ...props
}: ComponentProps<typeof PaginationLink>) {
  return (
    <PaginationLink
      aria-label="Go to next page"
      className={cn('w-auto gap-1 px-3', className)}
      {...props}
    >
      <span className="hidden sm:block">Next</span>
      <ChevronRightIcon size={16} strokeWidth={1.5} aria-hidden="true" />
    </PaginationLink>
  );
}

function PaginationEllipsis({ className, ...props }: ComponentProps<'span'>) {
  return (
    <span
      aria-hidden="true"
      data-slot="pagination-ellipsis"
      className={cn(
        'flex size-10 items-center justify-center text-text-faint',
        className,
      )}
      {...props}
    >
      <MoreHorizontalIcon size={16} strokeWidth={1.5} />
      <span className="sr-only">More pages</span>
    </span>
  );
}

export {
  Pagination,
  PaginationContent,
  PaginationLink,
  PaginationItem,
  PaginationPrevious,
  PaginationNext,
  PaginationEllipsis,
};
