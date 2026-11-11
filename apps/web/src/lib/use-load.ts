'use client';

import { useCallback, useEffect, useRef, useState } from 'react';

import { ApiError } from './api';

export type Load<T> =
  | { status: 'loading' }
  | { status: 'ready'; data: T }
  | { status: 'error'; error: ApiError };

/**
 * The three-case loading state of DESIGN.md section 11, in one place: loads
 * once when the screen opens and again when `retry` is called, and drops an
 * answer that arrives after the screen has gone. A dashboard with several
 * panels calls it once per panel, so one failing call never blanks the rest.
 */
export function useLoad<T>(load: (signal: AbortSignal) => Promise<T>) {
  const [state, setState] = useState<Load<T>>({ status: 'loading' });
  const [attempt, setAttempt] = useState(0);
  const loader = useRef(load);

  useEffect(() => {
    loader.current = load;
  });

  useEffect(() => {
    const controller = new AbortController();
    setState({ status: 'loading' });
    loader
      .current(controller.signal)
      .then((data) => {
        if (!controller.signal.aborted) setState({ status: 'ready', data });
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        setState({
          status: 'error',
          error:
            error instanceof ApiError
              ? error
              : new ApiError({ code: 'INTERNAL', status: 0 }),
        });
      });
    return () => controller.abort();
  }, [attempt]);

  const retry = useCallback(() => setAttempt((count) => count + 1), []);
  return { state, retry };
}
