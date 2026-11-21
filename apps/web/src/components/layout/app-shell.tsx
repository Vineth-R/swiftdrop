'use client';

import { Menu, PanelLeftClose, PanelLeftOpen } from 'lucide-react';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { useEffect, useRef, useState, type ReactNode } from 'react';

import type { User } from '../../lib/auth-api';
import { cn } from '../../lib/cn';
import { findNav, navByRole } from '../../lib/nav';
import { Logo, LogoMark } from '../ui/logo';
import { ThemeSwitch } from '../ui/theme-switch';
import { Drawer } from './drawer';
import { NavList } from './nav-list';
import { UserMenu } from './user-menu';

const SIDEBAR_KEY = 'sd-sidebar';

/** "/admin/zones" becomes "Zones", for a page that is not in the navigation. */
function titleFromPath(pathname: string): string {
  const last = pathname.split('/').filter(Boolean).pop() ?? '';
  const words = last.replace(/[-_]+/g, ' ').trim();
  return words ? words[0].toUpperCase() + words.slice(1) : 'SwiftDrop';
}

type AppShellProps = {
  user: User;
  /** A page-level notice between the top bar and the page, such as "pending approval". */
  banner?: ReactNode;
  children: ReactNode;
};

/**
 * The frame for customer, merchant, dispatcher and admin screens: a sidebar, a
 * 60px top bar and the content. The courier has its own shell.
 *
 * Widths: under 768px the navigation is a drawer opened from the top bar; from
 * 768px it is a 72px icon rail; from 1024px it expands to 264px, and the top
 * bar button folds it back to the rail. The choice is remembered.
 */
export function AppShell({ user, banner, children }: AppShellProps) {
  const pathname = usePathname();
  const [isCollapsed, setIsCollapsed] = useState(false);
  const [isDrawerOpen, setIsDrawerOpen] = useState(false);
  const navButtonRef = useRef<HTMLButtonElement>(null);
  const groups = navByRole[user.role];
  const current = findNav(user.role, pathname);

  useEffect(() => {
    try {
      setIsCollapsed(window.localStorage.getItem(SIDEBAR_KEY) === 'rail');
    } catch {
      // Storage can be blocked; the sidebar simply starts expanded.
    }
  }, []);

  useEffect(() => setIsDrawerOpen(false), [pathname]);

  function toggleSidebar() {
    const next = !isCollapsed;
    setIsCollapsed(next);
    try {
      window.localStorage.setItem(SIDEBAR_KEY, next ? 'rail' : 'expanded');
    } catch {
      // Remembering is a convenience only.
    }
  }

  const title = current?.item.label ?? titleFromPath(pathname);

  return (
    <div className="flex min-h-screen bg-paper">
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-4 focus:z-40 focus:rounded-control focus:bg-surface focus:px-4 focus:py-2 focus:text-label"
      >
        Skip to the content
      </a>

      {/* Sidebar: a rail from 768px, expanded from 1024px unless folded. */}
      <aside
        data-collapsed={isCollapsed}
        className={cn(
          'sticky top-0 hidden h-screen shrink-0 flex-col border-r border-line bg-surface md:flex',
          isCollapsed ? 'w-18' : 'w-18 lg:w-66',
        )}
      >
        <div
          className={cn(
            'flex h-15 shrink-0 items-center border-b border-line',
            isCollapsed
              ? 'justify-center'
              : 'justify-center lg:justify-start lg:px-4',
          )}
        >
          <Link
            href="/dashboard"
            aria-label="SwiftDrop home"
            className="text-text no-underline"
          >
            <span className={cn(isCollapsed ? 'inline' : 'inline lg:hidden')}>
              <LogoMark size={28} isDecorative />
            </span>
            <span className={cn(isCollapsed ? 'hidden' : 'hidden lg:inline')}>
              <Logo size="sm" />
            </span>
          </Link>
        </div>
        <nav aria-label="Main" className="min-h-0 flex-1 overflow-y-auto p-2">
          <div className={cn(isCollapsed ? 'block' : 'block lg:hidden')}>
            <NavList groups={groups} pathname={pathname} mode="rail" />
          </div>
          <div className={cn(isCollapsed ? 'hidden' : 'hidden lg:block')}>
            <NavList groups={groups} pathname={pathname} mode="full" />
          </div>
        </nav>
      </aside>

      <Drawer
        isOpen={isDrawerOpen}
        onClose={() => setIsDrawerOpen(false)}
        label="Navigation"
        returnFocusRef={navButtonRef}
      >
        <nav aria-label="Main">
          <NavList
            groups={groups}
            pathname={pathname}
            mode="touch"
            onNavigate={() => setIsDrawerOpen(false)}
          />
        </nav>
      </Drawer>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-20 flex h-15 items-center gap-3 border-b border-line bg-surface px-3 md:px-4">
          <button
            ref={navButtonRef}
            type="button"
            aria-label="Open navigation"
            onClick={() => setIsDrawerOpen(true)}
            className="inline-flex size-9 pointer-coarse:size-11 shrink-0 items-center justify-center rounded-control hover:bg-surface-2 md:hidden"
          >
            <Menu size={20} strokeWidth={1.5} aria-hidden="true" />
          </button>
          <button
            type="button"
            aria-label={
              isCollapsed ? 'Expand navigation' : 'Collapse navigation'
            }
            aria-expanded={!isCollapsed}
            onClick={toggleSidebar}
            className="hidden size-9 pointer-coarse:size-11 shrink-0 items-center justify-center rounded-control hover:bg-surface-2 lg:inline-flex"
          >
            {isCollapsed ? (
              <PanelLeftOpen size={20} strokeWidth={1.5} aria-hidden="true" />
            ) : (
              <PanelLeftClose size={20} strokeWidth={1.5} aria-hidden="true" />
            )}
          </button>

          <div className="flex min-w-0 flex-1 flex-col">
            {current?.group && (
              <nav
                aria-label="Breadcrumb"
                className="text-caption text-text-faint"
              >
                {current.group.label}
              </nav>
            )}
            <span className="truncate text-body font-semibold tracking-[-0.005em]">
              {title}
            </span>
          </div>

          <div className="hidden md:block">
            <ThemeSwitch variant="compact" />
          </div>
          <UserMenu user={user} />
        </header>

        {banner}

        <main
          id="main"
          tabIndex={-1}
          className="mx-auto flex w-full max-w-content flex-1 flex-col gap-12 px-4 py-8 outline-none md:px-8 md:py-10"
        >
          {children}
        </main>
      </div>
    </div>
  );
}
