'use client';

import { ChevronDown, LogOut, User as UserIcon, KeyRound } from 'lucide-react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import {
  useCallback,
  useEffect,
  useId,
  useRef,
  useState,
  type KeyboardEvent,
} from 'react';

import type { User } from '../../lib/auth-api';
import { cn } from '../../lib/cn';
import { useSession } from '../../lib/session';
import { Avatar } from '../ui/avatar';
import { RoleBadge } from '../ui/role-badge';
import { ThemeSwitch } from '../ui/theme-switch';

const itemClass =
  'flex h-10 w-full items-center gap-3 rounded-chip px-3 text-button font-medium text-text outline-offset-[-2px] hover:bg-surface-2 focus-visible:bg-surface-2';

/**
 * The signed-in person: who they are, their role, where to change their account
 * and how to leave. On a shared dispatch machine the most useful thing a menu
 * can say is who is signed in, so that comes first.
 *
 * Keyboard: Enter or Space opens it, the arrow keys move, Home and End jump,
 * Escape closes it and returns focus to the button.
 */
export function UserMenu({ user }: { user: User }) {
  const router = useRouter();
  const { signOut } = useSession();
  const [isOpen, setIsOpen] = useState(false);
  const [isLeaving, setIsLeaving] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const menuId = useId();

  const items = useCallback(
    () =>
      Array.from(
        rootRef.current?.querySelectorAll<HTMLElement>('[role="menuitem"]') ??
          [],
      ),
    [],
  );

  const close = useCallback((returnFocus: boolean) => {
    setIsOpen(false);
    if (returnFocus) triggerRef.current?.focus();
  }, []);

  useEffect(() => {
    if (!isOpen) return;
    items()[0]?.focus();
    function onPointerDown(event: PointerEvent) {
      if (!rootRef.current?.contains(event.target as Node)) close(false);
    }
    document.addEventListener('pointerdown', onPointerDown);
    return () => document.removeEventListener('pointerdown', onPointerDown);
  }, [isOpen, items, close]);

  function onMenuKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const all = items();
    const index = all.indexOf(document.activeElement as HTMLElement);
    if (event.key === 'Escape') {
      event.preventDefault();
      close(true);
    } else if (event.key === 'ArrowDown') {
      event.preventDefault();
      all[(index + 1) % all.length]?.focus();
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      all[(index - 1 + all.length) % all.length]?.focus();
    } else if (event.key === 'Home') {
      event.preventDefault();
      all[0]?.focus();
    } else if (event.key === 'End') {
      event.preventDefault();
      all[all.length - 1]?.focus();
    }
  }

  async function leave() {
    setIsLeaving(true);
    try {
      await signOut();
    } catch {
      // The server could not be reached; this browser still leaves the signed-in screens.
    }
    router.replace('/signed-out');
  }

  return (
    <div ref={rootRef} className="relative">
      <button
        ref={triggerRef}
        type="button"
        aria-haspopup="menu"
        aria-expanded={isOpen}
        aria-controls={isOpen ? menuId : undefined}
        aria-label="Account menu"
        onClick={() => setIsOpen((open) => !open)}
        className="inline-flex h-10 items-center gap-2 rounded-control border border-transparent pl-1 pr-2 hover:bg-surface-2 md:pr-3"
      >
        <Avatar name={user.fullName} />
        <span className="hidden max-w-40 truncate text-button font-medium md:inline">
          {user.fullName}
        </span>
        <ChevronDown
          size={16}
          strokeWidth={1.5}
          aria-hidden="true"
          className={cn(
            'text-text-muted transition-transform duration-fast ease-out-sd',
            isOpen && 'rotate-180',
          )}
        />
      </button>

      {isOpen && (
        <div
          id={menuId}
          role="menu"
          aria-label="Account"
          onKeyDown={onMenuKeyDown}
          className="absolute right-0 top-full z-30 mt-2 w-72 rounded-card border border-line bg-surface p-1 shadow-popover"
        >
          <div className="flex flex-col gap-2 px-3 py-3">
            <div className="flex flex-col">
              <span className="truncate text-label font-semibold">
                {user.fullName}
              </span>
              <span className="truncate text-caption text-text-muted">
                {user.email}
              </span>
            </div>
            <RoleBadge role={user.role} size="sm" />
          </div>
          <div role="none" className="my-1 h-px bg-line" />
          <Link
            href="/profile"
            role="menuitem"
            tabIndex={-1}
            onClick={() => close(false)}
            className={itemClass}
          >
            <UserIcon size={16} strokeWidth={1.5} aria-hidden="true" />
            Profile
          </Link>
          <Link
            href="/profile/security"
            role="menuitem"
            tabIndex={-1}
            onClick={() => close(false)}
            className={itemClass}
          >
            <KeyRound size={16} strokeWidth={1.5} aria-hidden="true" />
            Change password
          </Link>
          {/* The top bar carries the theme switch from 768px; below that it lives here. */}
          <div className="md:hidden">
            <div role="none" className="my-1 h-px bg-line" />
            <div className="flex items-center justify-between gap-3 px-3 py-2">
              <span className="text-button font-medium">Theme</span>
              <ThemeSwitch variant="compact" />
            </div>
          </div>
          <div role="none" className="my-1 h-px bg-line" />
          <button
            type="button"
            role="menuitem"
            tabIndex={-1}
            disabled={isLeaving}
            onClick={leave}
            className={cn(itemClass, 'text-danger')}
          >
            <LogOut size={16} strokeWidth={1.5} aria-hidden="true" />
            {isLeaving ? 'Logging out' : 'Log out'}
          </button>
        </div>
      )}
    </div>
  );
}
