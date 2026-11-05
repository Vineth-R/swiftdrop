import type { ReactNode } from 'react';
import Link from 'next/link';

import { cn } from '@/lib/cn';
import { Logo } from '@/components/ui/logo';
import { RouteLine } from '@/components/ui/route-line';
import { ThemeSwitch } from '@/components/ui/theme-switch';

type AuthLayoutProps = {
  /** A brand panel. Hidden below 768px, where only the header remains. */
  panel: ReactNode;
  /** Width of the form column: login and reset are narrower than register. */
  width?: 'narrow' | 'wide';
  /** The form column's content. */
  children: ReactNode;
};

const cities = ['Colombo', 'Kandy', 'Galle', 'Jaffna', 'Negombo', 'Matara'];

/**
 * A brand panel and a form. One column under 768px, where the panel shrinks to
 * a header with the logo, a short route line and the theme switch.
 *
 * The panel is Ink in both themes, so it opts into the dark token values with
 * data-theme="dark": text, route and line colours then read correctly on it
 * even when the page itself is light. The form side follows the theme.
 */
export function AuthLayout({
  panel,
  width = 'narrow',
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
        className="relative isolate overflow-clip bg-ink text-text md:flex-1 md:basis-96"
      >
        {/* On wide screens the panel stays in view while a long form scrolls. */}
        <div className="flex flex-col px-5 py-6 md:sticky md:top-0 md:h-screen md:p-10 xl:p-12">
          <div className="flex items-center justify-between gap-4">
            <Link
              href="/"
              aria-label="SwiftDrop home"
              className="text-text no-underline"
            >
              <Logo />
            </Link>
            <div className="md:hidden">
              <ThemeSwitch variant="compact" />
            </div>
          </div>
          <RouteLine state="complete" nodeSize={8} className="mt-4 md:hidden" />
          <div className="hidden min-h-0 flex-1 flex-col pt-8 md:flex">
            {panel}
          </div>
        </div>
      </aside>

      <main
        id="main"
        className="relative flex flex-1 flex-col md:basis-120 md:border-l md:border-line"
      >
        <div className="absolute right-6 top-6 hidden md:block">
          <ThemeSwitch variant="compact" />
        </div>
        <div className="flex flex-1 items-start justify-center px-4 py-10 md:items-center md:px-6 md:py-16">
          <div
            className={cn(
              'flex w-full flex-col gap-6',
              width === 'narrow' ? 'max-w-100' : 'max-w-115',
            )}
          >
            {children}
          </div>
        </div>
        <footer className="flex flex-col items-center gap-3 px-4 pb-8 md:px-6">
          <RouteLine state="idle" nodeSize={7} className="w-24" />
          <p className="text-balance text-center text-overline uppercase text-text-faint">
            {cities.join(' · ')}
          </p>
        </footer>
      </main>
    </div>
  );
}

/**
 * The form column's heading. The eyebrow names the moment (welcome back, new
 * account) in the small caps the system uses for labels; the line under the
 * heading is the one thing the person needs to read before the fields.
 */
export function AuthHeading({
  title,
  eyebrow,
  children,
}: {
  title: string;
  eyebrow?: string;
  children?: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-2">
      {eyebrow && (
        <span className="text-overline uppercase text-text-faint">
          {eyebrow}
        </span>
      )}
      <h1 className="font-display text-h2 md:text-h1">{title}</h1>
      {children && <p className="text-body text-text-muted">{children}</p>}
    </div>
  );
}
