/** Joins class names, skipping anything falsy. Variants never overlap, so no merge step is needed. */
export function cn(...parts: Array<string | false | null | undefined>): string {
  return parts.filter(Boolean).join(' ');
}
