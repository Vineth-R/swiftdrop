'use client';

import { useEffect, useState } from 'react';

/**
 * Counts whole seconds down from `seconds` and never goes below 0. Passing
 * null stops it and returns 0.
 */
export function useCountdown(seconds: number | null): number {
  const [deadline, setDeadline] = useState<number | null>(null);
  const [now, setNow] = useState(0);

  useEffect(() => {
    if (seconds === null) {
      setDeadline(null);
      return;
    }
    const start = Date.now();
    setDeadline(start + seconds * 1000);
    setNow(start);
    const timer = window.setInterval(() => setNow(Date.now()), 250);
    return () => window.clearInterval(timer);
  }, [seconds]);

  // Before the effect has set a deadline, the count is the full amount, so a
  // caller that redirects at 0 never fires on the first render.
  if (deadline === null) return seconds ?? 0;
  return Math.max(0, Math.ceil((deadline - now) / 1000));
}

/** 38 becomes "0:38", 125 becomes "2:05". */
export function formatClock(totalSeconds: number): string {
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}:${String(seconds).padStart(2, '0')}`;
}
