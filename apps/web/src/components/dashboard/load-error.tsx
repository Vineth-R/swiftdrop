import type { ApiError } from '@/lib/api';
import { Alert } from '../ui/alert';
import { Button } from '../ui/button';

/**
 * What a dashboard panel shows when its own call failed: what happened, what
 * to do, and the Reference only for a fault on our side. Never the server's words.
 */
export function LoadError({
  title,
  error,
  onRetry,
}: {
  title: string;
  error: ApiError;
  onRetry: () => void;
}) {
  return (
    <Alert
      tone="danger"
      title={title}
      actions={
        <Button variant="secondary" size="sm" onClick={onRetry}>
          Try again
        </Button>
      }
    >
      {error.isServerFault
        ? 'We could not reach SwiftDrop. Check your connection and try again.'
        : 'This could not be shown right now. Try again in a moment.'}
      {error.isServerFault && error.requestId ? (
        <span className="mt-1 block font-mono text-caption">
          Reference {error.requestId}
        </span>
      ) : null}
    </Alert>
  );
}
