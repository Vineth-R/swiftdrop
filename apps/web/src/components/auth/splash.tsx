'use client';

import { useEffect, useState } from 'react';

import { Button } from '../ui/button';
import { LogoMark } from '../ui/logo';
import { RouteLine } from '../ui/route-line';

type SplashProps = {
  /**
   * checking: the session cookie is being checked, normally for under a second.
   * unavailable: the check could not reach the API.
   */
  state?: 'checking' | 'unavailable';
  onRetry?: () => void;
};

/**
 * Shown only while the session is being checked. It states what is happening
 * instead of showing a fake progress bar. After 3 seconds it admits the wait
 * is slower than usual.
 */
export function Splash({ state = 'checking', onRetry }: SplashProps) {
  const [isSlow, setIsSlow] = useState(false);

  useEffect(() => {
    if (state !== 'checking') return;
    const timer = window.setTimeout(() => setIsSlow(true), 3000);
    return () => window.clearTimeout(timer);
  }, [state]);

  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-10 bg-paper px-4">
      <div className="flex flex-col items-center gap-5">
        <LogoMark size={76} radius={12} />
        <span className="font-display text-h1 font-semibold tracking-wordmark">
          SwiftDrop
        </span>
      </div>

      {state === 'checking' ? (
        <div className="flex flex-col items-center gap-3">
          <RouteLine
            state="travelling"
            cycleMs={2400}
            className="w-64 max-w-full"
          />
          <p role="status" className="text-body-sm text-text-muted">
            {isSlow
              ? 'Still working — this is slower than usual.'
              : 'Checking your session'}
          </p>
        </div>
      ) : (
        <div className="flex flex-col items-center gap-4 text-center">
          <RouteLine state="idle" className="w-64 max-w-full" />
          <h1 className="text-h4">We could not reach SwiftDrop</h1>
          <p className="max-w-sm text-body-sm text-text-muted">
            Check your connection and try again. Nothing you typed has been
            lost.
          </p>
          {onRetry && (
            <Button variant="secondary" onClick={onRetry}>
              Try again
            </Button>
          )}
        </div>
      )}
    </main>
  );
}
