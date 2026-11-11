'use client';

import { useState } from 'react';
import { ApiError } from '../lib/api';
import type { ProofPhotoTarget } from '../lib/parcels-api';
import { Button } from './ui/button';

type State = 'idle' | 'loading' | 'missing' | 'error';

/**
 * Fetches a short-lived proof photo target and opens it. The URL is never
 * persisted, and local storage uses the authorized content route.
 */
export function ProofPhotoButton({
  load,
}: {
  load: (signal?: AbortSignal) => Promise<ProofPhotoTarget>;
}) {
  const [state, setState] = useState<State>('idle');

  async function open() {
    if (state === 'loading') return;
    // Open synchronously so the browser keeps the user activation. Fetching
    // first and opening afterwards is blocked as a popup.
    const popup = window.open('about:blank', '_blank');
    setState('loading');
    try {
      const target = await load();
      if (popup) {
        popup.opener = null;
        popup.location.href = target.url;
      } else {
        window.location.assign(target.url);
      }
      setState('idle');
    } catch (error) {
      popup?.close();
      if (error instanceof ApiError && error.status === 404) {
        setState('missing');
      } else {
        setState('error');
      }
    }
  }

  return (
    <div className="flex flex-col items-start gap-2">
      <Button
        type="button"
        variant="secondary"
        onClick={() => void open()}
        isLoading={state === 'loading'}
      >
        {state === 'loading' ? 'Opening photo' : 'View proof photo'}
      </Button>
      {state === 'missing' && (
        <p role="status" className="text-body-sm text-text-muted">
          No proof photo is available for this delivery.
        </p>
      )}
      {state === 'error' && (
        <p role="alert" className="text-body-sm text-danger">
          The proof photo could not be opened. Try again.
        </p>
      )}
    </div>
  );
}
