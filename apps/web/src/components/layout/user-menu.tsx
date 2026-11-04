'use client';

import {
  ChevronDown,
  KeyRound,
  LogOut,
  Monitor,
  Moon,
  Sun,
  User as UserIcon,
  type LucideIcon,
} from 'lucide-react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useState, type KeyboardEvent } from 'react';

import type { User } from '../../lib/auth-api';
import { cn } from '../../lib/utils';
import { useSession } from '../../lib/session';
import type { ThemePreference } from '../../lib/theme';
import { useThemePreference } from '../../lib/use-theme-preference';
import { Avatar } from '../ui/avatar';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '../ui/dropdown-menu';
import { RoleBadge } from '../ui/role-badge';

const themeOptions: {
  value: ThemePreference;
  label: string;
  Icon: LucideIcon;
}[] = [
  { value: 'light', label: 'Light', Icon: Sun },
  { value: 'dark', label: 'Dark', Icon: Moon },
  { value: 'system', label: 'System', Icon: Monitor },
];

/**
 * The signed-in person: who they are, their role, where to change their account
 * and how to leave. On a shared dispatch machine the most useful thing a menu
 * can say is who is signed in, so that comes first.
 *
 * Keyboard behaviour comes from Radix's menu primitive: Enter or Space opens it,
 * the arrow keys move, Home and End jump, Escape closes it and returns focus.
 */
export function UserMenu({ user }: { user: User }) {
  const router = useRouter();
  const { signOut } = useSession();
  const { preference, choose, sync } = useThemePreference();
  const [isLeaving, setIsLeaving] = useState(false);

  async function leave() {
    setIsLeaving(true);
    try {
      await signOut();
    } catch {
      // The server could not be reached; this browser still leaves the signed-in screens.
    }
    router.replace('/signed-out');
  }

  function wrapMenuKeys(event: KeyboardEvent<HTMLDivElement>) {
    const items = Array.from(
      event.currentTarget.querySelectorAll<HTMLElement>(
        '[role="menuitem"]:not([data-disabled])',
      ),
    );
    if (items.length === 0) return;
    const index = items.indexOf(document.activeElement as HTMLElement);
    if (event.key === 'ArrowDown' && index === items.length - 1) {
      event.preventDefault();
      items[0].focus();
    } else if (event.key === 'ArrowUp' && index === 0) {
      event.preventDefault();
      items[items.length - 1].focus();
    }
  }

  return (
    <DropdownMenu
      modal={false}
      onOpenChange={(open) => {
        if (open) sync();
      }}
    >
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label="Account menu"
          className="group inline-flex h-10 items-center gap-2 rounded-control border border-transparent pl-1 pr-2 hover:bg-surface-2 md:pr-3"
        >
          <Avatar name={user.fullName} />
          <span className="hidden max-w-40 truncate text-button font-medium md:inline">
            {user.fullName}
          </span>
          <ChevronDown
            size={16}
            strokeWidth={1.5}
            aria-hidden="true"
            className="text-text-muted transition-transform duration-fast ease-out-sd group-data-[state=open]:rotate-180"
          />
        </button>
      </DropdownMenuTrigger>

      <DropdownMenuContent
        align="end"
        aria-label="Account"
        onKeyDownCapture={wrapMenuKeys}
        className="w-72 rounded-card border-line bg-surface p-1 shadow-popover"
      >
        <DropdownMenuLabel className="flex flex-col gap-2 px-3 py-3">
          <span className="flex flex-col">
            <span className="truncate text-label font-semibold">
              {user.fullName}
            </span>
            <span className="truncate text-caption text-text-muted">
              {user.email}
            </span>
          </span>
          <RoleBadge role={user.role} size="sm" />
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem asChild>
          <Link href="/profile">
            <UserIcon size={16} strokeWidth={1.5} aria-hidden="true" />
            Profile
          </Link>
        </DropdownMenuItem>
        <DropdownMenuItem asChild>
          <Link href="/profile/security">
            <KeyRound size={16} strokeWidth={1.5} aria-hidden="true" />
            Change password
          </Link>
        </DropdownMenuItem>
        {/* The top bar carries the theme switch from 768px; below that it lives here. */}
        <div className="md:hidden">
          <DropdownMenuSeparator />
          <DropdownMenuLabel className="px-3 py-2 text-caption text-text-muted">
            Theme
          </DropdownMenuLabel>
          <DropdownMenuRadioGroup
            value={preference}
            onValueChange={(value) => choose(value as ThemePreference)}
          >
            {themeOptions.map(({ value, label, Icon }) => (
              <DropdownMenuRadioItem
                key={value}
                value={value}
                className="h-10 rounded-chip text-button"
              >
                <Icon size={16} strokeWidth={1.5} aria-hidden="true" />
                {label}
              </DropdownMenuRadioItem>
            ))}
          </DropdownMenuRadioGroup>
        </div>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          variant="destructive"
          disabled={isLeaving}
          onSelect={() => {
            void leave();
          }}
          className={cn(isLeaving && 'cursor-progress')}
        >
          <LogOut size={16} strokeWidth={1.5} aria-hidden="true" />
          {isLeaving ? 'Logging out' : 'Log out'}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
