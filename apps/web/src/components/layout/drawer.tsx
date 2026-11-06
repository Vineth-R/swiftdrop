'use client';

import { X } from 'lucide-react';
import type { ReactNode, RefObject } from 'react';

import { Logo } from '../ui/logo';
import { Sheet, SheetContent, SheetTitle } from '../ui/sheet';

type DrawerProps = {
  isOpen: boolean;
  onClose: () => void;
  /** The accessible name, since the drawer has no visible heading of its own. */
  label: string;
  children: ReactNode;
  /** Focus returns here when the drawer closes, including on Escape. */
  returnFocusRef?: RefObject<HTMLElement | null>;
};

/**
 * Navigation that slides in from the left under 768px. Radix's dialog primitive
 * traps focus inside it, closes it on Escape, makes the page behind it inert and
 * hands focus back to the button that opened it. The scrim is a plain colour,
 * never a blur.
 */
export function Drawer({
  isOpen,
  onClose,
  label,
  children,
  returnFocusRef,
}: DrawerProps) {
  return (
    <Sheet open={isOpen} onOpenChange={(open) => !open && onClose()}>
      <SheetContent
        side="left"
        showCloseButton={false}
        aria-label={label}
        onCloseAutoFocus={(event) => {
          if (returnFocusRef?.current) {
            event.preventDefault();
            returnFocusRef.current.focus();
          }
        }}
        className="w-60 max-w-none gap-0 border-r border-line bg-surface p-0 text-text data-[state=open]:animate-drawer-in"
      >
        <SheetTitle className="sr-only">{label}</SheetTitle>
        <div className="flex h-15 shrink-0 items-center justify-between border-b border-line pl-4 pr-3">
          <Logo size="sm" />
          <button
            type="button"
            aria-label="Close navigation"
            onClick={onClose}
            className="inline-flex size-9 items-center justify-center rounded-control hover:bg-surface-2"
          >
            <X size={20} strokeWidth={1.5} aria-hidden="true" />
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto p-2">{children}</div>
      </SheetContent>
    </Sheet>
  );
}
