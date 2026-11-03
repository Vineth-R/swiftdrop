import Link from 'next/link';

import { isActive, type NavGroup } from '../../lib/nav';
import { cn } from '../../lib/cn';

type NavListProps = {
  groups: NavGroup[];
  pathname: string;
  /**
   * full: icon and label. rail: icon only, the label stays for screen readers
   * and appears as a tooltip. touch: taller rows for the drawer.
   */
  mode: 'full' | 'rail' | 'touch';
  onNavigate?: () => void;
};

/**
 * The sidebar's links. The active item gets a 3px lime rail on its left edge,
 * Ink text and a Surface-2 fill, so it is never marked by colour alone.
 */
export function NavList({ groups, pathname, mode, onNavigate }: NavListProps) {
  const isRail = mode === 'rail';
  return (
    <div className="flex flex-col gap-1">
      {groups.map((group, index) => (
        <div
          key={group.label ?? index}
          role="group"
          aria-label={group.label}
          className={cn('flex flex-col gap-0.5', index > 0 && 'mt-3')}
        >
          {group.label &&
            (isRail ? (
              <div
                role="presentation"
                className="mx-2 mb-1 border-t border-line"
              />
            ) : (
              <div className="flex items-center justify-between px-3 pb-1.5 pt-2.5">
                <span className="text-overline uppercase text-text-faint">
                  {group.label}
                </span>
                {group.badge && (
                  <span className="rounded-chip bg-primary px-1.5 py-0.5 text-[9px] font-semibold tracking-[0.06em] text-on-primary">
                    {group.badge}
                  </span>
                )}
              </div>
            ))}
          {group.items.map((item) => {
            const active = isActive(item.href, pathname);
            return (
              <Link
                key={item.href}
                href={item.href}
                aria-current={active ? 'page' : undefined}
                title={isRail ? item.label : undefined}
                onClick={onNavigate}
                className={cn(
                  'flex items-center gap-2.5 rounded-control px-2.5 text-button no-underline transition-colors duration-fast ease-out-sd hover:bg-surface-2',
                  mode === 'touch' ? 'h-12' : 'h-10',
                  isRail && 'justify-center',
                  active
                    ? 'bg-surface-2 font-semibold text-text hover:text-text'
                    : 'font-medium text-text-muted hover:text-text',
                )}
              >
                <span
                  aria-hidden="true"
                  className={cn(
                    'h-5 w-0.75 shrink-0 rounded-chip',
                    active ? 'bg-accent' : 'bg-transparent',
                  )}
                />
                <item.icon
                  size={20}
                  strokeWidth={1.5}
                  aria-hidden="true"
                  className="shrink-0"
                />
                {isRail ? (
                  <span className="sr-only">{item.label}</span>
                ) : (
                  <span className="flex-1 truncate">{item.label}</span>
                )}
                {!isRail && item.count !== undefined && (
                  <span className="tabular font-mono text-body-sm text-text-muted">
                    {item.count}
                  </span>
                )}
              </Link>
            );
          })}
        </div>
      ))}
    </div>
  );
}
