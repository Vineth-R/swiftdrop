import { notFound } from 'next/navigation';

/**
 * Catches every signed-in path that has no page of its own, so the 404 appears
 * inside the shell (see ../not-found.tsx). Real pages take precedence over this.
 */
export default function UnknownPage(): never {
  notFound();
}
