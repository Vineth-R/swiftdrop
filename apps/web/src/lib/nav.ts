import type { Role } from './tokens';

/**
 * Where each role lands after signing in. Every role goes to /dashboard until
 * its own home screen exists; change the entry for a role when you build that
 * role's first screen, for example DISPATCHER to '/unassigned'.
 */
const roleHome: Record<Role, string> = {
  CUSTOMER: '/dashboard',
  MERCHANT: '/dashboard',
  COURIER: '/dashboard',
  DISPATCHER: '/dashboard',
  ADMIN: '/dashboard',
};

export function homeFor(role: Role): string {
  return roleHome[role];
}
