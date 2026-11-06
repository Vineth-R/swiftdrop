import { cn } from '@/lib/cn';

type SpinnerProps = {
  size?: 16 | 20;
  /**
   * on-primary: lime node, for the Ink button. current: the node takes the
   * text colour, for every other button and link. standalone: lime node with
   * an Ink edge, for page-level loading.
   */
  tone?: 'current' | 'on-primary' | 'standalone';
  /** Spoken label. Leave empty inside a button that already says what is happening. */
  label?: string;
  className?: string;
};

/** A dotted ring with one node running around it, instead of an arc sweep. */
export function Spinner({
  size = 16,
  tone = 'current',
  label,
  className,
}: SpinnerProps) {
  const standalone = tone === 'standalone';
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 16 16"
      className={cn('shrink-0', className)}
      {...(label
        ? { role: 'img', 'aria-label': label }
        : { 'aria-hidden': true })}
    >
      <circle
        cx="8"
        cy="8"
        r="5.6"
        fill="none"
        strokeWidth="1.5"
        strokeDasharray="1.6 2.6"
        className={standalone ? 'stroke-line' : 'stroke-current opacity-35'}
      />
      <circle
        data-spinner-node=""
        cx="8"
        cy="2.4"
        r="2.1"
        strokeWidth={standalone ? 0.8 : 0}
        className={cn(
          'animate-node-spin origin-center [transform-box:view-box]',
          tone === 'current' ? 'fill-current' : 'fill-accent',
          standalone && 'stroke-ink',
        )}
      />
    </svg>
  );
}
