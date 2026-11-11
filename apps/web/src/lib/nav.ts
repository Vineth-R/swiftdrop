import {
  ChartColumn,
  CircleDollarSign,
  FileText,
  History,
  KeyRound,
  LayoutDashboard,
  List,
  Map,
  Package,
  PackagePlus,
  ScrollText,
  Store,
  Truck,
  User,
  Users,
  type LucideIcon,
} from 'lucide-react';

import { DEFAULT_AUTH_ROUTE } from './constants';
import type { Role } from './tokens';

/**
 * Where each role lands after signing in. Every role goes to /dashboard until
 * its own home screen exists; change the entry for a role when you build that
 * role's first screen, for example DISPATCHER to '/unassigned'.
 */
const roleHome: Record<Role, string> = {
  CUSTOMER: DEFAULT_AUTH_ROUTE,
  MERCHANT: DEFAULT_AUTH_ROUTE,
  COURIER: DEFAULT_AUTH_ROUTE,
  DISPATCHER: '/unassigned',
  ADMIN: DEFAULT_AUTH_ROUTE,
};

export function homeFor(role: Role): string {
  return roleHome[role];
}

export type NavItem = {
  label: string;
  href: string;
  icon: LucideIcon;
  /** A live number shown at the end of the row, such as unassigned parcels. */
  count?: number;
};

export type NavGroup = {
  /** A small caps label above the group. Omit it for a role with one group. */
  label?: string;
  /** Marks a group only some roles may see, such as "ADMIN". */
  badge?: string;
  items: NavItem[];
};

const dispatch: NavGroup = {
  label: 'Dispatch',
  items: [
    { label: 'Unassigned', href: '/unassigned', icon: Package },
    { label: 'Workload', href: '/workload', icon: ChartColumn },
    { label: 'All parcels', href: '/parcels', icon: List },
    { label: 'Reports', href: '/reports', icon: FileText },
  ],
};

/**
 * What each role's sidebar (or, for the courier, bottom tab bar) holds.
 *
 * The navigation is built from the role on the session, not from the URL, and
 * hiding an item never protects a route: every page still has to check the
 * role itself (see RequireRole). Add an entry here when you build a screen.
 * Pages that are not built yet show the shell's "does not exist" page.
 */
export const navByRole: Record<Role, NavGroup[]> = {
  CUSTOMER: [
    {
      items: [
        { label: 'Dashboard', href: '/dashboard', icon: LayoutDashboard },
        { label: 'New delivery', href: '/new', icon: PackagePlus },
        { label: 'My parcels', href: '/parcels', icon: Package },
        { label: 'Profile', href: '/profile', icon: User },
      ],
    },
  ],
  MERCHANT: [
    {
      items: [
        { label: 'Dashboard', href: '/dashboard', icon: LayoutDashboard },
        { label: 'API keys', href: '/api-keys', icon: KeyRound },
        { label: 'Business', href: '/merchant/profile', icon: Store },
        { label: 'Profile', href: '/profile', icon: User },
      ],
    },
  ],
  // The courier gets bottom tabs instead of a sidebar. "Today" opens the
  // courier dashboard with the day's assigned deliveries.
  COURIER: [
    {
      items: [
        { label: 'Today', href: '/dashboard', icon: Truck },
        { label: 'History', href: '/history', icon: History },
        { label: 'Profile', href: '/profile', icon: User },
      ],
    },
  ],
  DISPATCHER: [dispatch],
  // An admin sees every dispatcher screen plus four of their own, in a second
  // labelled group, so a screen-sharing dispatcher can tell which are not theirs.
  ADMIN: [
    dispatch,
    {
      label: 'Administration',
      badge: 'ADMIN',
      items: [
        { label: 'Users', href: '/admin/users', icon: Users },
        { label: 'Merchants', href: '/admin/merchants', icon: Store },
        { label: 'Zones', href: '/admin/zones', icon: Map },
        { label: 'Pricing', href: '/admin/pricing', icon: CircleDollarSign },
        {
          label: 'Notification logs',
          href: '/admin/notification-logs',
          icon: ScrollText,
        },
      ],
    },
  ],
};

/** True when `pathname` is the item's page or a page below it. */
export function isActive(href: string, pathname: string): boolean {
  return pathname === href || pathname.startsWith(`${href}/`);
}

/** The group and item a path belongs to, for the top bar's breadcrumb and title. */
export function findNav(
  role: Role,
  pathname: string,
): { group?: NavGroup; item: NavItem } | null {
  for (const group of navByRole[role]) {
    // The longest matching href wins, so /admin/users beats a shorter prefix.
    const match = group.items
      .filter((item) => isActive(item.href, pathname))
      .sort((a, b) => b.href.length - a.href.length)[0];
    if (match) return { group: group.label ? group : undefined, item: match };
  }
  return null;
}
