export const MAX_PROOF_PHOTO_BYTES = 5 * 1024 * 1024;

const PHOTO_TYPES = new Set(['image/jpeg', 'image/png']);

export type ProofErrors = {
  form?: string;
  photo?: string;
  recipientName?: string;
  note?: string;
};

export type ProofPhoto = {
  type: string;
  size: number;
};

export type ProofDraft = {
  photo: ProofPhoto | null;
  recipientName: string;
  note: string;
};

export function proofPhotoError(photo: ProofPhoto): string | undefined {
  if (!PHOTO_TYPES.has(photo.type)) return 'Choose a JPEG or PNG image.';
  if (photo.size > MAX_PROOF_PHOTO_BYTES)
    return 'Photo must be 5 MiB or smaller.';
  return undefined;
}

export function validateProofDraft({
  photo,
  recipientName,
  note,
}: ProofDraft): ProofErrors {
  const errors: ProofErrors = {};
  if (!photo && !recipientName.trim() && !note.trim())
    errors.form = 'Add a photo, recipient name or delivery note.';
  if (Array.from(recipientName.trim()).length > 150)
    errors.recipientName = 'Recipient name must be 150 characters or fewer.';
  if (Array.from(note.trim()).length > 500)
    errors.note = 'Delivery note must be 500 characters or fewer.';
  return errors;
}

export function isProofLimitReached(
  error: { code: string; status: number } | null | undefined,
): boolean {
  return (
    error?.code === 'PROOF_LIMIT_REACHED' ||
    (error?.code === 'CONFLICT' && error.status === 409)
  );
}
