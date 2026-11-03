import { cn } from '../../lib/cn';

/** Up to two initials from a person's name: "Nimali Perera" gives "NP". */
export function initialsOf(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean);
  if (words.length === 0) return '?';
  const first = words[0][0];
  const last = words.length > 1 ? words[words.length - 1][0] : '';
  return (first + last).toUpperCase();
}

const sizes = {
  sm: 'size-7 text-overline',
  md: 'size-8 text-caption',
} as const;

type AvatarProps = {
  name: string;
  size?: keyof typeof sizes;
  className?: string;
};

/** Initials on an Ink circle. It is decorative: the name is always written next to it. */
export function Avatar({ name, size = 'md', className }: AvatarProps) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        'inline-flex shrink-0 items-center justify-center rounded-full bg-primary font-semibold text-on-primary',
        sizes[size],
        className,
      )}
    >
      {initialsOf(name)}
    </span>
  );
}
