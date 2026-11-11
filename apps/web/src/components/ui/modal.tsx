'use client';

import { X } from 'lucide-react';
import { useEffect, useId, useRef, type ReactNode } from 'react';

import { cn } from '../../lib/cn';

type ModalProps = {
  isOpen: boolean;
  onClose: () => void;
  /** The question or the task, in one short line. It is the dialog's name. */
  title: string;
  /** One sentence that says what will happen. */
  description?: ReactNode;
  children?: ReactNode;
  /** The footer buttons: one primary (or danger) action and a secondary Cancel. */
  actions?: ReactNode;
  /**
   * False removes the close button and makes Escape and a click outside do
   * nothing. Use it only while a request is running or when the person must choose.
   */
  isDismissible?: boolean;
  size?: 'sm' | 'md';
};

/**
 * A dialog that asks for a decision or a small piece of input, such as the
 * reason for rejecting a merchant. It is a native dialog, so the browser traps
 * focus inside it, closes it on Escape, makes the page behind it inert and
 * hands focus back to the control that opened it. Put `data-autofocus` on the
 * field or the safe button that should receive focus first (React's `autoFocus`
 * does nothing in a dialog that is mounted while closed); without one, the
 * close button does. It is the only kit piece besides a popover that carries a
 * shadow.
 */
export function Modal({
  isOpen,
  onClose,
  title,
  description,
  children,
  actions,
  isDismissible = true,
  size = 'md',
}: ModalProps) {
  const ref = useRef<HTMLDialogElement>(null);
  const isClosingFromProp = useRef(false);
  const titleId = useId();
  const descriptionId = useId();

  useEffect(() => {
    const dialog = ref.current;
    if (!dialog) return;
    if (isOpen && !dialog.open) {
      dialog.showModal();
      dialog.querySelector<HTMLElement>('[data-autofocus]')?.focus();
    }
    if (!isOpen && dialog.open) {
      // Closing because the parent said so must not tell the parent again.
      isClosingFromProp.current = true;
      dialog.close();
    }
  }, [isOpen]);

  return (
    <dialog
      ref={ref}
      aria-labelledby={titleId}
      aria-describedby={description ? descriptionId : undefined}
      onClose={() => {
        if (isClosingFromProp.current) {
          isClosingFromProp.current = false;
          return;
        }
        onClose();
      }}
      onCancel={(event) => {
        // Escape. A modal that must be answered ignores it.
        if (!isDismissible) event.preventDefault();
      }}
      onClick={(event) => {
        // The dialog fills the screen, so a click on its own element is a click outside the panel.
        if (isDismissible && event.target === ref.current) onClose();
      }}
      className="fixed inset-0 m-0 hidden h-dvh max-h-none w-full max-w-none items-center justify-center border-0 bg-transparent p-4 text-text open:flex backdrop:bg-scrim"
    >
      <div
        className={cn(
          'flex max-h-full w-full flex-col gap-4 overflow-y-auto rounded-card border border-line bg-surface p-6 shadow-modal',
          size === 'sm' ? 'max-w-sm' : 'max-w-md',
        )}
      >
        <div className="flex items-start justify-between gap-4">
          <h2 id={titleId} className="text-h3">
            {title}
          </h2>
          {isDismissible && (
            <button
              type="button"
              aria-label="Close"
              onClick={onClose}
              className="-mr-2 -mt-1 inline-flex size-9 pointer-coarse:size-11 shrink-0 items-center justify-center rounded-control text-text-muted hover:bg-surface-2"
            >
              <X size={20} strokeWidth={1.5} aria-hidden="true" />
            </button>
          )}
        </div>
        {description && (
          <p id={descriptionId} className="text-body text-text-muted">
            {description}
          </p>
        )}
        {children}
        {actions && (
          <div className="flex flex-col-reverse gap-3 md:flex-row md:justify-end">
            {actions}
          </div>
        )}
      </div>
    </dialog>
  );
}
