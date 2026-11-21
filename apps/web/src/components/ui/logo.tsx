import { cn } from '@/lib/cn';

type LogoMarkProps = {
  /** Rendered size in px. Below 24 the dotted route is hard to read; use the favicon. */
  size?: number;
  /** 10 for lockups, 12 for the splash screen. */
  radius?: 10 | 12;
  className?: string;
  /** Hide from assistive tech when the wordmark sits next to the mark. */
  isDecorative?: boolean;
};

/**
 * The SwiftDrop mark: a point of origin and a point of arrival, joined by a
 * dotted line. The tile follows the theme (Ink in light, paper-dark in dark)
 * and the lime node is the only lime element.
 */
export function LogoMark({
  size = 30,
  radius = 10,
  className,
  isDecorative = false,
}: LogoMarkProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 48 48"
      className={cn('shrink-0', className)}
      {...(isDecorative
        ? { 'aria-hidden': true }
        : { role: 'img', 'aria-label': 'SwiftDrop' })}
    >
      <rect width="48" height="48" rx={radius} className="fill-primary" />
      <path
        d="M13 35 L29.5 18.5"
        className="stroke-on-primary"
        strokeWidth="1.5"
        strokeDasharray="2.5 3.5"
        strokeLinecap="round"
        fill="none"
      />
      <circle cx="13" cy="35" r="2.4" className="fill-on-primary" />
      <circle cx="32" cy="16" r="4.2" className="fill-accent" />
    </svg>
  );
}

const wordmarkSize = {
  sm: 'text-h4',
  md: 'text-h3',
  lg: 'text-display',
} as const;

const markSize = { sm: 26, md: 30, lg: 44 } as const;

type LogoProps = {
  size?: keyof typeof wordmarkSize;
  className?: string;
};

/** Mark and wordmark together. The wordmark is Bricolage Grotesque 600 at -0.028em. */
export function Logo({ size = 'md', className }: LogoProps) {
  return (
    <span className={cn('inline-flex items-center gap-3', className)}>
      <LogoMark size={markSize[size]} isDecorative />
      <span
        className={cn(
          'font-display font-semibold tracking-wordmark',
          wordmarkSize[size],
        )}
      >
        SwiftDrop
      </span>
    </span>
  );
}
