import { Spinner } from '../ui/spinner';

/** The one looping motion allowed here is the kit's spinner, not bouncing dots. */
export function TypingIndicator() {
  return (
    <div className="flex items-center gap-2 px-1 text-caption text-text-muted">
      <Spinner size={16} />
      <span>The assistant is typing</span>
    </div>
  );
}
