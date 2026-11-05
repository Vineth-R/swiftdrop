'use client';

import { Monitor, Moon, Sun, type LucideIcon } from 'lucide-react';

import { cn } from '@/lib/utils';
import type { ThemePreference } from '@/lib/theme';
import { useThemePreference } from '@/lib/use-theme-preference';

const options: { value: ThemePreference; label: string; Icon: LucideIcon }[] = [
  { value: 'light', label: 'Light', Icon: Sun },
  { value: 'dark', label: 'Dark', Icon: Moon },
  { value: 'system', label: 'System', Icon: Monitor },
];

type ThemeSwitchProps = {
  /**
   * full: icon and word, for a settings page or menu.
   * compact: icons only, for a corner of a page. Each button still carries its
   * name for screen readers.
   */
  variant?: 'full' | 'compact';
  className?: string;
};

export function ThemeSwitch({ variant = 'full', className }: ThemeSwitchProps) {
  const { preference, choose } = useThemePreference();
  const isCompact = variant === 'compact';

  return (
    <div
      role="group"
      aria-label="Theme"
      className={cn(
        'inline-flex gap-1 rounded-control border border-line-control bg-surface p-1',
        className,
      )}
    >
      {options.map(({ value, label, Icon }) => (
        <button
          key={value}
          type="button"
          aria-pressed={preference === value}
          aria-label={isCompact ? label : undefined}
          title={isCompact ? label : undefined}
          onClick={() => choose(value)}
          className={cn(
            'inline-flex h-8 items-center justify-center rounded-chip text-label text-text-muted transition-colors duration-fast ease-out-sd hover:bg-surface-2 aria-pressed:bg-surface-2 aria-pressed:text-text',
            isCompact ? 'w-8' : 'gap-2 px-3',
          )}
        >
          <Icon size={16} strokeWidth={1.5} aria-hidden="true" />
          {!isCompact && label}
        </button>
      ))}
    </div>
  );
}
