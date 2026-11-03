import { buttonClasses } from '../ui/button';
import { cn } from '../../lib/cn';

/* eslint-disable swiftdrop/no-raw-colors -- Google's own brand colours: the mark must not follow our palette. */
function GoogleMark() {
  return (
    <svg width="18" height="18" viewBox="0 0 18 18" aria-hidden="true">
      <path
        d="M17.64 9.2c0-.64-.06-1.25-.16-1.84H9v3.48h4.84a4.14 4.14 0 0 1-1.8 2.72v2.26h2.92a8.78 8.78 0 0 0 2.68-6.62z"
        fill="#4285F4"
      />
      <path
        d="M9 18c2.43 0 4.47-.8 5.96-2.18l-2.92-2.26c-.8.54-1.84.86-3.04.86-2.34 0-4.32-1.58-5.03-3.7H.96v2.34A9 9 0 0 0 9 18z"
        fill="#34A853"
      />
      <path
        d="M3.97 10.72a5.4 5.4 0 0 1 0-3.44V4.94H.96a9 9 0 0 0 0 8.12l3.01-2.34z"
        fill="#FBBC05"
      />
      <path
        d="M9 3.58c1.32 0 2.5.46 3.44 1.35l2.58-2.59C13.46.9 11.43 0 9 0A9 9 0 0 0 .96 4.94l3.01 2.34C4.68 5.16 6.66 3.58 9 3.58z"
        fill="#EA4335"
      />
    </svg>
  );
}
/* eslint-enable swiftdrop/no-raw-colors */

/**
 * Starts Google sign-in. A plain link, because the API answers with a redirect
 * to Google and sets a cookie that ties the attempt to this browser. It keeps
 * Google's own mark and never takes the Ink or accent fill.
 */
export function GoogleButton({
  href,
  children,
}: {
  href: string;
  children: string;
}) {
  return (
    <a
      href={href}
      className={cn(
        buttonClasses({ variant: 'secondary', size: 'lg', isFullWidth: true }),
        'no-underline',
      )}
    >
      <GoogleMark />
      {children}
    </a>
  );
}

export function OrDivider() {
  return (
    <div className="flex items-center gap-3" role="separator" aria-label="or">
      <span className="h-px flex-1 bg-line" />
      <span className="text-overline uppercase text-text-faint">or</span>
      <span className="h-px flex-1 bg-line" />
    </div>
  );
}
