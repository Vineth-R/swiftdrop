'use client';

import { Monitor, Moon, Sun, type LucideIcon } from 'lucide-react';
import { useEffect, useState } from 'react';

import {
  applyPreference,
  readPreference,
  type ThemePreference,
} from '../../lib/theme';

const options: { value: ThemePreference; label: string; Icon: LucideIcon }[] = [
  { value: 'light', label: 'Light', Icon: Sun },
  { value: 'dark', label: 'Dark', Icon: Moon },
  { value: 'system', label: 'System', Icon: Monitor },
];

export function ThemeSwitch() {
  const [preference, setPreference] = useState<ThemePreference>('system');

  useEffect(() => {
    setPreference(readPreference());
  }, []);

  useEffect(() => {
    if (preference !== 'system') return;
    const query = window.matchMedia('(prefers-color-scheme: dark)');
    const follow = () => applyPreference('system');
    query.addEventListener('change', follow);
    return () => query.removeEventListener('change', follow);
  }, [preference]);

  function choose(next: ThemePreference) {
    setPreference(next);
    applyPreference(next);
  }

  return (
    <div
      role="group"
      aria-label="Theme"
      className="inline-flex gap-1 rounded-control border border-line-control bg-surface p-1"
    >
      {options.map(({ value, label, Icon }) => (
        <button
          key={value}
          type="button"
          aria-pressed={preference === value}
          onClick={() => choose(value)}
          className="inline-flex h-8 items-center gap-2 rounded-chip px-3 text-label text-text-muted transition-colors duration-fast ease-out-sd hover:bg-surface-2 aria-pressed:bg-surface-2 aria-pressed:text-text"
        >
          <Icon size={16} strokeWidth={1.5} aria-hidden="true" />
          {label}
        </button>
      ))}
    </div>
  );
}
