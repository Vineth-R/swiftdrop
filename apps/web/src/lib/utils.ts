import { clsx, type ClassValue } from 'clsx';
import { extendTailwindMerge } from 'tailwind-merge';

/**
 * SwiftDrop's type scale uses names such as text-body-sm that tailwind-merge
 * cannot infer. Register them as font sizes so they survive when a colour
 * utility follows in the same class list.
 */
const twMerge = extendTailwindMerge({
  extend: {
    classGroups: {
      'font-size': [
        {
          text: [
            'display',
            'h1',
            'h2',
            'h3',
            'h4',
            'body',
            'body-sm',
            'body-lg',
            'label',
            'caption',
            'overline',
            'button',
          ],
        },
      ],
    },
  },
});

/** Join class names and resolve conflicting Tailwind utilities. */
export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs));
}
