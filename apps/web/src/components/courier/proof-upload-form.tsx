'use client';

import Image from 'next/image';
import { useEffect, useRef, useState, type FormEvent } from 'react';

import { ApiError } from '../../lib/api';
import { uploadCourierDeliveryProof } from '../../lib/courier-api';
import { useSession } from '../../lib/session';
import { ServerFaultAlert } from '../auth/form-alerts';
import { Alert } from '../ui/alert';
import { Button } from '../ui/button';
import { TextField } from '../ui/field';
import { FileField } from '../ui/file-field';
import { Textarea } from '../ui/textarea';

const MAX_PHOTO_BYTES = 5 * 1024 * 1024;
const PHOTO_TYPES = new Set(['image/jpeg', 'image/png']);

type ProofErrors = {
  form?: string;
  photo?: string;
  recipientName?: string;
  note?: string;
};

export function ProofUploadForm({
  parcelId,
  onUploaded,
}: {
  parcelId: string;
  onUploaded: () => void;
}) {
  const { recheck } = useSession();
  const request = useRef<AbortController | null>(null);
  const [photo, setPhoto] = useState<File | null>(null);
  const [preview, setPreview] = useState<string | null>(null);
  const [recipientName, setRecipientName] = useState('');
  const [note, setNote] = useState('');
  const [errors, setErrors] = useState<ProofErrors>({});
  const [uploadError, setUploadError] = useState<ApiError | null>(null);
  const [uploading, setUploading] = useState(false);
  const [pickerKey, setPickerKey] = useState(0);

  useEffect(() => () => request.current?.abort(), []);
  useEffect(() => {
    if (!photo) {
      setPreview(null);
      return;
    }
    const url = URL.createObjectURL(photo);
    setPreview(url);
    return () => URL.revokeObjectURL(url);
  }, [photo]);

  function choosePhoto(selected: File | null) {
    setUploadError(null);
    if (!selected) {
      setPhoto(null);
      setErrors((current) => ({ ...current, photo: undefined }));
      return;
    }
    if (!PHOTO_TYPES.has(selected.type)) {
      setPickerKey((key) => key + 1);
      setPhoto(null);
      setErrors((current) => ({
        ...current,
        photo: 'Choose a JPEG or PNG image.',
      }));
      return;
    }
    if (selected.size > MAX_PHOTO_BYTES) {
      setPickerKey((key) => key + 1);
      setPhoto(null);
      setErrors((current) => ({
        ...current,
        photo: 'Photo must be 5 MiB or smaller.',
      }));
      return;
    }
    setPhoto(selected);
    setErrors((current) => ({ ...current, form: undefined, photo: undefined }));
  }

  function validate(): ProofErrors {
    const next: ProofErrors = {};
    if (!photo && !recipientName.trim() && !note.trim())
      next.form = 'Add a photo, recipient name or delivery note.';
    if (Array.from(recipientName.trim()).length > 150)
      next.recipientName = 'Recipient name must be 150 characters or fewer.';
    if (Array.from(note.trim()).length > 500)
      next.note = 'Delivery note must be 500 characters or fewer.';
    setErrors(next);
    return next;
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (uploading || request.current) return;
    const nextErrors = validate();
    const first = Object.keys(nextErrors)[0] as keyof ProofErrors | undefined;
    if (first) {
      document
        .getElementById(
          first === 'form' ? 'courier-proof-photo' : `courier-proof-${first}`,
        )
        ?.focus();
      return;
    }

    const controller = new AbortController();
    request.current = controller;
    setUploading(true);
    setUploadError(null);
    try {
      await uploadCourierDeliveryProof(
        parcelId,
        {
          photo: photo ?? undefined,
          recipientName: recipientName.trim() || undefined,
          note: note.trim() || undefined,
        },
        controller.signal,
      );
      if (!controller.signal.aborted) onUploaded();
    } catch (caught) {
      if (controller.signal.aborted) return;
      const next =
        caught instanceof ApiError
          ? caught
          : new ApiError({ code: 'INTERNAL', status: 0 });
      if (
        next.code === 'ACCOUNT_DEACTIVATED' ||
        next.code === 'UNAUTHENTICATED'
      )
        recheck();
      setUploadError(next);
    } finally {
      request.current = null;
      if (!controller.signal.aborted) setUploading(false);
    }
  }

  const blocked =
    uploading ||
    uploadError?.code === 'FORBIDDEN' ||
    uploadError?.code === 'NOT_FOUND';

  return (
    <form noValidate onSubmit={submit} className="flex max-w-xl flex-col gap-4">
      {errors.form && (
        <Alert tone="danger" title="Add delivery proof">
          {errors.form}
        </Alert>
      )}
      {uploadError && <ProofUploadError error={uploadError} />}
      <FileField
        key={pickerKey}
        id="courier-proof-photo"
        label="Proof photo"
        helper="JPEG or PNG, up to 5 MiB."
        error={errors.photo}
        name="photo"
        accept="image/jpeg,image/png"
        buttonLabel="Take or choose a photo"
        emptyLabel="No photo selected"
        controlSize="lg"
        disabled={blocked}
        onFileChange={choosePhoto}
      />
      {photo && (
        <div className="flex flex-col gap-3" aria-live="polite">
          <p className="break-all text-body-sm text-text-muted">
            Selected: {photo.name}
          </p>
          {preview && (
            <Image
              src={preview}
              alt="Selected proof preview"
              width={480}
              height={320}
              unoptimized
              className="max-h-64 w-full rounded-card border border-line object-contain"
            />
          )}
        </div>
      )}
      <TextField
        id="courier-proof-recipientName"
        name="recipientName"
        label="Proof received by"
        helper="Optional, up to 150 characters."
        controlSize="lg"
        maxLength={150}
        value={recipientName}
        error={errors.recipientName}
        disabled={blocked}
        onChange={(event) => {
          setRecipientName(event.target.value);
          setErrors((current) => ({
            ...current,
            form: undefined,
            recipientName: undefined,
          }));
        }}
      />
      <Textarea
        id="courier-proof-note"
        name="note"
        label="Delivery note"
        helper="Optional, up to 500 characters."
        controlSize="lg"
        maxLength={500}
        showCount
        value={note}
        error={errors.note}
        disabled={blocked}
        onChange={(event) => {
          setNote(event.target.value);
          setErrors((current) => ({
            ...current,
            form: undefined,
            note: undefined,
          }));
        }}
      />
      <Button
        type="submit"
        size="lg"
        className="self-start"
        isLoading={uploading}
        isDisabled={blocked}
      >
        {uploading ? 'Uploading proof' : 'Upload proof'}
      </Button>
    </form>
  );
}

function ProofUploadError({ error }: { error: ApiError }) {
  if (error.code === 'UNAUTHENTICATED' || error.code === 'ACCOUNT_DEACTIVATED')
    return null;
  if (error.code === 'FORBIDDEN')
    return (
      <Alert tone="danger" title="You no longer have access to this delivery">
        Return to Today and choose a delivery currently assigned to you.
      </Alert>
    );
  if (error.code === 'NOT_FOUND')
    return (
      <Alert tone="danger" title="Delivery not found">
        Return to Today and choose an assigned delivery.
      </Alert>
    );
  if (error.status === 413)
    return (
      <Alert tone="danger" title="Photo is too large">
        Choose a JPEG or PNG image no larger than 5 MiB.
      </Alert>
    );
  if (error.status === 415)
    return (
      <Alert tone="danger" title="Photo format is not supported">
        Choose a valid JPEG or PNG image.
      </Alert>
    );
  if (error.status === 400 || error.status === 422)
    return (
      <Alert tone="danger" title="Proof details need review">
        Check the selected photo and proof fields, then try again.
      </Alert>
    );
  if (error.status === 503)
    return (
      <Alert tone="danger" title="Proof uploads are temporarily unavailable">
        Your input is still here. Try again shortly.
      </Alert>
    );
  if (error.isServerFault)
    return (
      <ServerFaultAlert
        isNetwork={error.code === 'NETWORK'}
        requestId={error.requestId}
      />
    );
  return (
    <Alert tone="danger" title="Proof could not be uploaded">
      Your input is still here. Review it and try again.
    </Alert>
  );
}
