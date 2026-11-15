'use client';

import type { ReactNode } from 'react';

import { Alert } from './alert';
import { Button } from './button';
import { Modal } from './modal';

type ConfirmationDialogProps = {
  isOpen: boolean;
  onClose: () => void;
  title: string;
  description: ReactNode;
  confirmLabel: string;
  cancelLabel?: string;
  /** Danger for destructive actions; primary for an approval. */
  tone?: 'danger' | 'primary';
  onConfirm: () => void;
  isLoading?: boolean;
  error?: ReactNode;
  children?: ReactNode;
};

/**
 * A short decision that needs one more click. It keeps the safe choice first
 * in the tab order for destructive actions and never closes while the action
 * is running.
 */
export function ConfirmationDialog({
  isOpen,
  onClose,
  title,
  description,
  confirmLabel,
  cancelLabel = 'Cancel',
  tone = 'danger',
  onConfirm,
  isLoading = false,
  error,
  children,
}: ConfirmationDialogProps) {
  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={title}
      description={description}
      size="sm"
      isDismissible={!isLoading}
      actions={
        <>
          <Button
            variant="secondary"
            data-autofocus
            isDisabled={isLoading}
            onClick={onClose}
          >
            {cancelLabel}
          </Button>
          <Button
            variant={tone === 'danger' ? 'danger' : 'primary'}
            isLoading={isLoading}
            onClick={onConfirm}
          >
            {confirmLabel}
          </Button>
        </>
      }
    >
      {error ? (
        <Alert tone="danger" title="Could not complete the action">
          {error}
        </Alert>
      ) : null}
      {children}
    </Modal>
  );
}
