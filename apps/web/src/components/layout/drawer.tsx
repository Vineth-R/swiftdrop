'use client';

import { X } from 'lucide-react';
import { useEffect, useRef, type ReactNode } from 'react';

import { Logo } from '../ui/logo';

type DrawerProps = {
  isOpen: boolean;
  onClose: () => void;
  /** The accessible name, since the drawer has no visible heading of its own. */
  label: string;
  children: ReactNode;
};

/**
 * Navigation that slides in from the left under 768px. It is a native dialog,
 * so the browser traps focus inside it, closes it on Escape, makes the page
 * behind it inert and hands focus back to the button that opened it. The
 * scrim is a plain colour, never a blur.
 */
export function Drawer({ isOpen, onClose, label, children }: DrawerProps) {
  const ref = useRef<HTMLDialogElement>(null);

  useEffect(() => {
    const dialog = ref.current;
    if (!dialog) return;
    if (isOpen && !dialog.open) dialog.showModal();
    if (!isOpen && dialog.open) dialog.close();
  }, [isOpen]);

  return (
    <dialog
      ref={ref}
      aria-label={label}
      onClose={onClose}
      onClick={(event) => {
        // A click on the scrim lands on the dialog element itself.
        if (event.target === ref.current) onClose();
      }}
      className="fixed inset-y-0 left-0 m-0 hidden h-dvh max-h-none w-60 max-w-none flex-col border-0 border-r border-line bg-surface p-0 text-text open:flex open:animate-drawer-in backdrop:bg-scrim"
    >
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
    </dialog>
  );
}
