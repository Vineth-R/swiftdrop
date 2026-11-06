'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import type { ReactNode } from 'react';

import type { User } from '../../lib/auth-api';
import { cn } from '../../lib/cn';
import { findNav, isActive, navByRole } from '../../lib/nav';
import { LogoMark } from '../ui/logo';
import { UserMenu } from './user-menu';

type CourierShellProps = {
  user: User;
  banner?: ReactNode;
  children: ReactNode;
};

/**
 * The courier's frame: mobile first, no sidebar. A slim top bar, and three
 * bottom tabs with 24px icons, 56px tall plus the phone's own safe-area inset.
 * Only the courier role gets bottom tabs; every other role gets the drawer.
 */
export function CourierShell({ user, banner, children }: CourierShellProps) {
  const pathname = usePathname();
  const items = navByRole.COURIER.flatMap((group) => group.items);
  const title = findNav('COURIER', pathname)?.item.label ?? 'SwiftDrop';

  return (
    <div className="flex min-h-screen flex-col bg-paper">
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-4 focus:z-40 focus:rounded-control focus:bg-surface focus:px-4 focus:py-2 focus:text-label"
      >
        Skip to the content
      </a>
      <header className="sticky top-0 z-20 flex h-14 items-center gap-3 border-b border-line bg-surface px-4">
        <Link href="/dashboard" aria-label="SwiftDrop home">
          <LogoMark size={28} isDecorative />
        </Link>
        <span className="flex-1 truncate text-body font-semibold">{title}</span>
        <UserMenu user={user} />
      </header>

      {banner}

      <main
        id="main"
        tabIndex={-1}
        className="mx-auto flex w-full max-w-content flex-1 flex-col gap-8 px-4 pb-28 pt-6 outline-none md:px-8"
      >
        {children}
      </main>

      <nav
        aria-label="Main"
        className="fixed inset-x-0 bottom-0 z-20 border-t border-line bg-surface pb-[env(safe-area-inset-bottom)]"
      >
        <ul className="mx-auto flex h-14 max-w-content">
          {items.map((item) => {
            const active = isActive(item.href, pathname);
            return (
              <li key={item.href} className="relative flex-1">
                {/* The lime rail sits on the top edge for the tab bar. */}
                <span
                  aria-hidden="true"
                  className={cn(
                    'absolute inset-x-6 top-0 h-0.75 rounded-b-chip',
                    active ? 'bg-accent' : 'bg-transparent',
                  )}
                />
                <Link
                  href={item.href}
                  aria-current={active ? 'page' : undefined}
                  className={cn(
                    'flex h-full flex-col items-center justify-center gap-0.5 text-caption no-underline',
                    active ? 'font-semibold text-text' : 'text-text-muted',
                  )}
                >
                  <item.icon size={24} strokeWidth={1.5} aria-hidden="true" />
                  {item.label}
                </Link>
              </li>
            );
          })}
        </ul>
      </nav>
    </div>
  );
}
