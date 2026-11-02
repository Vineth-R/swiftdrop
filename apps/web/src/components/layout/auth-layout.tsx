import type { ReactNode } from 'react';
import Link from 'next/link';

import { cn } from '../../lib/cn';
import { Logo } from '../ui/logo';
import { RouteLine } from '../ui/route-line';

type AuthLayoutProps = {
  /** What the dark panel says. Hidden below 768px, where only the header remains. */
  panel: ReactNode;
  /** Width of the form column: login and reset are narrower than register. */
  width?: 'narrow' | 'wide';
  /** Draw the route line along the bottom of the panel. The login panel has its own. */
  showRoute?: boolean;
  /** The form column's content. */
  children: ReactNode;
};

/**
 * Ink brand panel and a form. One column under 768px, where the panel shrinks
 * to a header with the logo and a short route line.
 *
 * The panel is Ink in both themes, so it opts into the dark token values with
 * data-theme="dark": text, route and line colours then read correctly on it
 * even when the page itself is light.
 */
export function AuthLayout({
  panel,
  width = 'narrow',
  showRoute = true,
  children,
}: AuthLayoutProps) {
  return (
    <div className="flex min-h-screen flex-col bg-paper md:flex-row">
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-4 focus:z-10 focus:rounded-control focus:bg-surface focus:px-4 focus:py-2 focus:text-label"
      >
        Skip to the form
      </a>

      <aside
        data-theme="dark"
        className="flex flex-col gap-4 bg-ink px-5 py-6 text-text md:flex-1 md:basis-96 md:justify-between md:gap-10 md:p-12"
      >
        <Link
          href="/"
          aria-label="SwiftDrop home"
          className="self-start text-text no-underline"
        >
          <Logo />
        </Link>
        <div className="hidden md:block">{panel}</div>
        <RouteLine state="complete" nodeSize={8} className="md:hidden" />
        {showRoute && (
          <RouteLine state="complete" className="hidden w-full md:flex" />
        )}
      </aside>

      <main
        id="main"
        className="flex flex-1 items-start justify-center px-4 py-10 md:basis-120 md:items-center md:px-6 md:py-12"
      >
        <div
          className={cn(
            'flex w-full flex-col gap-6',
            width === 'narrow' ? 'max-w-100' : 'max-w-115',
          )}
        >
          {children}
        </div>
      </main>
    </div>
  );
}

/** The form column's heading and its one line of help. */
export function AuthHeading({
  title,
  children,
}: {
  title: string;
  children?: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-2">
      <h1 className="font-display text-h2 md:text-h1">{title}</h1>
      {children && <p className="text-body text-text-muted">{children}</p>}
    </div>
  );
}
