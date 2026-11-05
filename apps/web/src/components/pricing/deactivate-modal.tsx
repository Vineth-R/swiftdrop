'use client';

import { Button } from '../ui/button';
import { Modal } from '../ui/modal';

/** Asks before something stops being used. Focus starts on the safe button. */
export function DeactivateModal({
  name,
  description,
  onClose,
  onConfirm,
}: {
  /** What is being deactivated, or null while the dialog is closed. */
  name: string | null;
  description: string;
  onClose: () => void;
  onConfirm: () => void;
}) {
  return (
    <Modal
      isOpen={name !== null}
      onClose={onClose}
      title={name ? `Deactivate ${name}?` : ''}
      description={description}
      actions={
        <>
          <Button
            variant="secondary"
            size="lg"
            data-autofocus
            onClick={onClose}
          >
            Keep active
          </Button>
          <Button variant="danger" size="lg" onClick={onConfirm}>
            Deactivate
          </Button>
        </>
      }
    />
  );
}
