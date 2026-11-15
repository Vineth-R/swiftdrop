import { proofPhotoError, type ProofPhoto } from './proof-upload';

export type WorkflowErrors = {
  form?: string;
  photo?: string;
  recipientName?: string;
  note?: string;
  reason?: string;
};

export type DeliveryDraft = {
  photo: ProofPhoto | null;
  recipientName: string;
  note: string;
};

/** Pickup needs the photo; the note is optional. */
export function validatePickup(photo: ProofPhoto | null): WorkflowErrors {
  const errors: WorkflowErrors = {};
  if (!photo) {
    errors.photo = 'Add a JPEG or PNG photo of the parcel at pickup.';
    return errors;
  }
  const problem = proofPhotoError(photo);
  if (problem) errors.photo = problem;
  return errors;
}

/** Delivery needs the recipient name and a photo; the note is optional. */
export function validateDelivery({
  photo,
  recipientName,
  note,
}: DeliveryDraft): WorkflowErrors {
  const errors: WorkflowErrors = {};
  const recipient = recipientName.trim();
  if (!recipient) {
    errors.recipientName =
      'Enter the name of the person who received the parcel.';
  } else if (Array.from(recipient).length > 150) {
    errors.recipientName = 'Recipient name must be 150 characters or fewer.';
  }
  if (!photo) {
    errors.photo = 'Add a JPEG or PNG photo of the handover.';
  } else {
    const problem = proofPhotoError(photo);
    if (problem) errors.photo = problem;
  }
  if (Array.from(note.trim()).length > 500) {
    errors.note = 'Delivery note must be 500 characters or fewer.';
  }
  return errors;
}

/** A failed delivery needs a clear reason of 5 to 255 characters. */
export function validateFailure(reason: string): string | undefined {
  const length = Array.from(reason.trim()).length;
  if (length < 5 || length > 255) {
    return 'Failure reason must be between 5 and 255 characters.';
  }
  return undefined;
}
