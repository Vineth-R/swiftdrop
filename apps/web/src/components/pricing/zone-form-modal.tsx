'use client';

import { useEffect, useState, type FormEvent } from 'react';

import type { ApiError } from '../../lib/api';
import { createZone, updateZone, type AdminZone } from '../../lib/pricing-api';
import { useForm, type FormRules } from '../../lib/use-form';
import { Button } from '../ui/button';
import { TextField } from '../ui/field';
import { Modal } from '../ui/modal';
import { FailureAlert, fieldErrors } from './failure-alert';
import { asApiError } from './use-paged-list';

type Values = { code: string; name: string; districts: string };

const FORM_ID = 'zone-form';
const fields = ['code', 'name', 'districts'] as const;

const rules: FormRules<Values> = {
  code: ({ code }) =>
    code.trim().length >= 1 && code.trim().length <= 20
      ? undefined
      : 'Enter a code of up to 20 characters.',
  name: ({ name }) =>
    name.trim().length >= 1 && name.trim().length <= 100
      ? undefined
      : 'Enter a name of up to 100 characters.',
  districts: () => undefined,
};

function districtsOf(text: string): string[] {
  return text
    .split(',')
    .map((district) => district.trim())
    .filter(Boolean);
}

/** Add a zone, or edit one. The API does not let a zone's code change. */
export function ZoneFormModal({
  isOpen,
  zone,
  onClose,
  onSaved,
}: {
  isOpen: boolean;
  /** The zone being edited, or null to add one. */
  zone: AdminZone | null;
  onClose: () => void;
  onSaved: (zone: AdminZone) => void;
}) {
  const form = useForm<Values>({ code: '', name: '', districts: '' }, rules);
  const { setValue, resetValidation, setServerErrors } = form;
  const [isSaving, setIsSaving] = useState(false);
  const [problem, setProblem] = useState<ApiError | null>(null);

  useEffect(() => {
    if (!isOpen) return;
    setValue('code', zone?.code ?? '');
    setValue('name', zone?.name ?? '');
    setValue('districts', zone?.districts.join(', ') ?? '');
    setServerErrors({});
    resetValidation();
    setProblem(null);
  }, [isOpen, zone, setValue, setServerErrors, resetValidation]);

  const name = form.values.name.trim();
  const districts = districtsOf(form.values.districts);
  const isNameChanged = zone !== null && name !== zone.name;
  const areDistrictsChanged =
    zone !== null && districts.join() !== zone.districts.join();
  const isDirty = zone === null || isNameChanged || areDistrictsChanged;

  function focusFirstBad(found: Partial<Record<keyof Values, string>>) {
    const first = fields.find((field) => found[field]);
    if (first) document.getElementById(`zone-${first}`)?.focus();
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isSaving || !isDirty) return;
    setProblem(null);
    const found = form.validate();
    if (Object.keys(found).length > 0) {
      focusFirstBad(found);
      return;
    }

    setIsSaving(true);
    try {
      const saved = zone
        ? await updateZone(zone.id, {
            ...(isNameChanged ? { name } : {}),
            ...(areDistrictsChanged ? { districts } : {}),
          })
        : await createZone({
            code: form.values.code.trim(),
            name,
            ...(districts.length > 0 ? { districts } : {}),
          });
      onSaved(saved);
    } catch (reason) {
      const error = asApiError(reason);
      const mapped =
        error.code === 'ZONE_CODE_EXISTS'
          ? {
              code: 'This code is already used by another zone. Choose a different code.',
            }
          : fieldErrors(error, fields);
      if (Object.keys(mapped).length > 0) {
        form.setServerErrors(mapped);
        focusFirstBad(mapped);
      } else {
        setProblem(error);
      }
    } finally {
      setIsSaving(false);
    }
  }

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      isDismissible={!isSaving}
      title={zone ? `Edit ${zone.name}` : 'Add a zone'}
      description={
        zone
          ? 'Change the name or districts. Parcels already stored keep their fee.'
          : 'A new zone can be used once it has pricing rules.'
      }
      actions={
        <>
          <Button
            variant="secondary"
            size="lg"
            isDisabled={isSaving}
            onClick={onClose}
          >
            Cancel
          </Button>
          <Button
            type="submit"
            form={FORM_ID}
            size="lg"
            isLoading={isSaving}
            isDisabled={!isDirty}
          >
            {isSaving ? 'Saving' : zone ? 'Save zone' : 'Add zone'}
          </Button>
        </>
      }
    >
      <form
        id={FORM_ID}
        noValidate
        onSubmit={submit}
        className="flex flex-col gap-4"
      >
        {problem && <FailureAlert error={problem} />}
        <TextField
          id="zone-code"
          label="Code"
          isMono
          isRequired
          controlSize="lg"
          maxLength={20}
          readOnly={zone !== null}
          helper={
            zone ? 'The code of a zone cannot be changed.' : 'For example GLE.'
          }
          data-autofocus={zone ? undefined : true}
          value={form.values.code}
          error={form.errors.code}
          onChange={(event) => form.setValue('code', event.target.value)}
          onBlur={() => form.blur('code')}
        />
        <TextField
          id="zone-name"
          label="Name"
          isRequired
          controlSize="lg"
          maxLength={100}
          data-autofocus={zone ? true : undefined}
          value={form.values.name}
          error={form.errors.name}
          onChange={(event) => form.setValue('name', event.target.value)}
          onBlur={() => form.blur('name')}
        />
        <TextField
          id="zone-districts"
          label="Districts"
          controlSize="lg"
          helper="Separate them with commas, for example Galle, Matara."
          value={form.values.districts}
          error={form.errors.districts}
          onChange={(event) => form.setValue('districts', event.target.value)}
          onBlur={() => form.blur('districts')}
        />
      </form>
    </Modal>
  );
}
