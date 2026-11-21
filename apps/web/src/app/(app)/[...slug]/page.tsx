import type { Metadata } from 'next';
import { notFound } from 'next/navigation';

// The tab title says what happened. Next uses this page's metadata for the 404
// it renders from here; a title set in not-found.tsx would be ignored.
export const metadata: Metadata = { title: 'Page not found — SwiftDrop' };

/**
 * Catches every signed-in path that has no page of its own, so the 404 appears
 * inside the shell (see ../not-found.tsx). Real pages take precedence over this.
 */
export default function UnknownPage(): never {
  notFound();
}
