import Link from 'next/link';
import type { ReactNode } from 'react';

import { cn } from '../../lib/cn';
import { ButtonLink } from '../ui/button';
import { Logo } from '../ui/logo';
import { ThemeSwitch } from '../ui/theme-switch';

type PublicLayoutProps = {
  /**
   * narrow is a reading column, right for tracking on a phone. wide fills the
   * content width, for a landing page.
   */
  width?: 'narrow' | 'wide';
  children: ReactNode;
};

/**
 * The frame of a page anyone can open, such as public tracking: a header with
 * the logo, the theme switch and a way to log in, and one column of content.
 * There is no session check and no sidebar. Whatever you put in it is public,
 * so show status, cities and times only, never names, phone numbers, addresses
 * or fees (rule 11). Build the page with `PageHeader` and `Section`.
 */
export function PublicLayout({
  width = 'narrow',
  children,
}: PublicLayoutProps) {
  return (
    <div className="flex min-h-screen flex-col bg-paper">
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-4 focus:z-40 focus:rounded-control focus:bg-surface focus:px-4 focus:py-2 focus:text-label"
      >
        Skip to the content
      </a>
      <header className="flex h-15 shrink-0 items-center justify-between gap-3 border-b border-line bg-surface px-4 md:px-8">
        <Link
          href="/"
          aria-label="SwiftDrop home"
          className="text-text no-underline"
        >
          <Logo size="sm" />
        </Link>
        <div className="flex items-center gap-3">
          <ThemeSwitch variant="compact" />
          <ButtonLink href="/login" variant="secondary" size="sm">
            Log in
          </ButtonLink>
        </div>
      </header>
      <main
        id="main"
        className={cn(
          'mx-auto flex w-full flex-1 flex-col gap-12 px-4 py-8 outline-none md:px-8 md:py-10',
          width === 'narrow' ? 'max-w-160' : 'max-w-content',
        )}
      >
        {children}
      </main>
    </div>
  );
}
