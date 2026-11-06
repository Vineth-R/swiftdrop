import { cn } from '@/lib/cn';

type ReferenceLineProps = {
  /** The API request ID. Nothing renders when it is missing. */
  requestId?: string;
  className?: string;
};

/** The "Reference …" line shown for faults on our side. Never use it for a 4xx. */
export function ReferenceLine({ requestId, className }: ReferenceLineProps) {
  if (!requestId) return null;
  return (
    <span
      className={cn(
        'select-all font-mono text-overline text-text-faint',
        className,
      )}
    >
      Reference {requestId}
    </span>
  );
}
