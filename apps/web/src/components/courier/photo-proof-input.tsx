'use client';

import Image from 'next/image';
import { useEffect, useState } from 'react';

import { FileField } from '../ui/file-field';

/**
 * One proof photo: the courier's file picker (which opens the camera on a
 * phone) and a local preview. The API still validates the decoded content;
 * this only helps the courier check the right photo before sending it.
 */
export function PhotoProofInput({
  id,
  label,
  helper,
  photo,
  error,
  disabled,
  onChange,
}: {
  id: string;
  label: string;
  helper: string;
  photo: File | null;
  error?: string;
  disabled: boolean;
  onChange: (file: File | null) => void;
}) {
  const [preview, setPreview] = useState<string | null>(null);

  useEffect(() => {
    if (!photo) {
      setPreview(null);
      return;
    }
    const url = URL.createObjectURL(photo);
    setPreview(url);
    return () => URL.revokeObjectURL(url);
  }, [photo]);

  return (
    <div className="flex flex-col gap-3">
      <FileField
        id={id}
        label={label}
        helper={helper}
        error={error}
        isRequired
        name="photo"
        accept="image/jpeg,image/png"
        capture="environment"
        buttonLabel="Take or choose a photo"
        emptyLabel="No photo selected"
        controlSize="lg"
        disabled={disabled}
        onFileChange={onChange}
      />
      {photo && preview && (
        <div className="flex flex-col gap-2" aria-live="polite">
          <Image
            src={preview}
            alt={`${label} preview`}
            width={480}
            height={320}
            unoptimized
            className="max-h-64 w-full rounded-card border border-line object-contain"
          />
        </div>
      )}
    </div>
  );
}
