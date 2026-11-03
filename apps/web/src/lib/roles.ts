import type { Role } from './tokens';

/**
 * Whether someone with role `actual` may do what `required` allows. An ADMIN
 * satisfies every DISPATCHER rule (API v1.1 section 1.3); every other role
 * only satisfies itself.
 */
export function roleSatisfies(actual: Role, required: Role): boolean {
  return (
    actual === required || (actual === 'ADMIN' && required === 'DISPATCHER')
  );
}

/** "DISPATCHER" becomes "a dispatcher", for a sentence about who someone is. */
export function describeRole(role: Role): string {
  const name = role.toLowerCase();
  return `${/^[aeiou]/.test(name) ? 'an' : 'a'} ${name}`;
}
