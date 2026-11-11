'use client';

import { useRouter } from 'next/navigation';

import { Button } from '../ui/button';
import { Modal } from '../ui/modal';

/**
 * Tells a signed-in person their session ended, without taking the page away
 * from them. It must be answered: there is nothing to dismiss, because every
 * request on the page would fail now. "Log in" opens the login page with this
 * page's address in `next`, query string included, so signing in brings them
 * back to the same place.
 *
 * `isOpen` is only for the style guide, which shows the dialog on demand. The
 * app shows it by mounting it, through `SignedInOnly`.
 */
export function SessionExpiredDialog({
  isOpen = true,
  onLogIn,
}: {
  isOpen?: boolean;
  /** Replaces the navigation, for the style guide. */
  onLogIn?: () => void;
}) {
  const router = useRouter();

  function logIn() {
    if (onLogIn) return onLogIn();
    const here = window.location.pathname + window.location.search;
    router.replace(`/login?next=${encodeURIComponent(here)}`);
  }

  return (
    <Modal
      isOpen={isOpen}
      onClose={() => undefined}
      isDismissible={false}
      size="sm"
      title="Your session ended"
      description="You were signed out. Log in again to carry on where you were."
      actions={
        <Button data-autofocus size="lg" onClick={logIn} isFullWidth>
          Log in
        </Button>
      }
    />
  );
}
